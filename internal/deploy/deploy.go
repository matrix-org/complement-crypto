package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement/must"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// must match the value in tests/addons/__init__.py
const magicMITMURL = "http://mitm.code"

type SlidingSyncDeployment struct {
	complement.Deployment
	postgres       testcontainers.Container
	slidingSync    testcontainers.Container
	reverseProxy   testcontainers.Container
	slidingSyncURL string
	mitmClient     *http.Client
	proxyURLToHS   map[string]string
	mu             sync.RWMutex
	tcpdump        *exec.Cmd
}

func (d *SlidingSyncDeployment) SlidingSyncURL(t *testing.T) string {
	t.Helper()
	if d.slidingSync == nil || d.slidingSyncURL == "" {
		t.Fatalf("SlidingSyncURL: not set")
		return ""
	}
	return d.slidingSyncURL
}

// WithMITMOptions changes the options of mitmproxy and executes inner() whilst those options are in effect.
// As the options on mitmproxy are a shared resource, this function has transaction-like semantics, ensuring
// the lock is released when inner() returns. This is similar to the `with` keyword in python.
func (d *SlidingSyncDeployment) WithMITMOptions(t *testing.T, options map[string]interface{}, inner func()) {
	t.Helper()
	lockID := d.lockOptions(t, options)
	defer d.unlockOptions(t, lockID)
	inner()
}

func (d *SlidingSyncDeployment) lockOptions(t *testing.T, options map[string]interface{}) (lockID []byte) {
	jsonBody, err := json.Marshal(map[string]interface{}{
		"options": options,
	})
	must.NotError(t, "failed to marshal options", err)
	u := magicMITMURL + "/options/lock"
	req, err := http.NewRequest("POST", u, bytes.NewBuffer(jsonBody))
	must.NotError(t, "failed to prepare request", err)
	req.Header.Set("Content-Type", "application/json")
	res, err := d.mitmClient.Do(req)
	must.NotError(t, "failed to POST "+u, err)
	must.Equal(t, res.StatusCode, 200, "controller returned wrong HTTP status")
	lockID, err = io.ReadAll(res.Body)
	must.NotError(t, "failed to read response", err)
	return lockID
}

func (d *SlidingSyncDeployment) unlockOptions(t *testing.T, lockID []byte) {
	req, err := http.NewRequest("POST", magicMITMURL+"/options/unlock", bytes.NewBuffer(lockID))
	must.NotError(t, "failed to prepare request", err)
	req.Header.Set("Content-Type", "application/json")
	res, err := d.mitmClient.Do(req)
	must.NotError(t, "failed to do request", err)
	must.Equal(t, res.StatusCode, 200, "controller returned wrong HTTP status")
}

func (d *SlidingSyncDeployment) ReverseProxyURLForHS(hsName string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.proxyURLToHS[hsName]
}

func (d *SlidingSyncDeployment) Teardown(writeLogs bool) {
	if d.slidingSync != nil {
		if writeLogs {
			err := writeContainerLogs(d.slidingSync, "container-sliding-sync.log")
			if err != nil {
				log.Printf("failed to write sliding sync logs: %s", err)
			}
		}
		if err := d.slidingSync.Terminate(context.Background()); err != nil {
			log.Fatalf("failed to stop sliding sync: %s", err)
		}
	}
	if d.postgres != nil {
		if err := d.postgres.Terminate(context.Background()); err != nil {
			log.Fatalf("failed to stop postgres: %s", err)
		}
	}
	if d.reverseProxy != nil {
		if writeLogs {
			err := writeContainerLogs(d.reverseProxy, "container-mitmproxy.log")
			if err != nil {
				log.Printf("failed to write sliding sync logs: %s", err)
			}
		}
		if err := d.reverseProxy.Terminate(context.Background()); err != nil {
			log.Fatalf("failed to stop reverse proxy: %s", err)
		}
	}
	if d.tcpdump != nil {
		fmt.Println("Sent SIGINT to tcpdump, waiting for it to exit, err=", d.tcpdump.Process.Signal(os.Interrupt))
		fmt.Println("tcpdump finished, err=", d.tcpdump.Wait())
	}
}

func RunNewDeployment(t *testing.T, shouldTCPDump bool) *SlidingSyncDeployment {
	// allow 30s for everything to deploy
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Deploy the homeserver using Complement
	deployment := complement.Deploy(t, 2)
	networkName := deployment.Network()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to find working directory: %s", err)
	}

	// Make the mitmproxy and hardcode CONTAINER PORTS for hs1/hs2. HOST PORTS are still dynamically allocated.
	// By running this container on the same network as the homeservers, we can leverage DNS hence hs1/hs2 URLs.
	// We also need to preload addons into the proxy, so we bind mount the addons directory. This also allows
	// test authors to easily add custom addons.
	hs1ExposedPort := "3000/tcp"
	hs2ExposedPort := "3001/tcp"
	controllerExposedPort := "8080/tcp" // default mitmproxy uses
	mitmproxyContainer, err := testcontainers.GenericContainer(context.Background(), testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "mitmproxy/mitmproxy:10.1.5",
			ExposedPorts: []string{hs1ExposedPort, hs2ExposedPort, controllerExposedPort},
			Env:          map[string]string{},
			Cmd: []string{
				"mitmdump",
				"--mode", "reverse:http://hs1:8008@3000",
				"--mode", "reverse:http://hs2:8008@3001",
				"--mode", "regular",
				"-s", "/addons/__init__.py",
			},
			WaitingFor: wait.ForLog("loading complement crypto addons"),
			Networks:   []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"mitmproxy"},
			},
			Mounts: testcontainers.Mounts(
				testcontainers.BindMount(filepath.Join(workingDir, "addons"), "/addons"),
			),
		},
		Started: true,
	})
	must.NotError(t, "failed to start reverse proxy container", err)
	rpHS1URL := externalURL(t, mitmproxyContainer, hs1ExposedPort)
	rpHS2URL := externalURL(t, mitmproxyContainer, hs2ExposedPort)
	controllerURL := externalURL(t, mitmproxyContainer, controllerExposedPort)

	// Make a postgres container
	postgresContainer, err := testcontainers.GenericContainer(context.Background(), testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:13-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "postgres",
				"POSTGRES_PASSWORD": "postgres",
				"POSTGRES_DB":       "syncv3",
			},
			WaitingFor: wait.ForExec([]string{"pg_isready"}).WithExitCodeMatcher(func(exitCode int) bool {
				fmt.Println("pg_isready exit code", exitCode)
				return exitCode == 0
			}).WithPollInterval(time.Second),
			Networks: []string{networkName},
			NetworkAliases: map[string][]string{
				networkName: {"postgres"},
			},
		},
		Started: true,
	})
	must.NotError(t, "failed to start postgres container", err)

	// Make a sliding sync proxy
	ssExposedPort := "6789/tcp"
	ssContainer, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "ghcr.io/matrix-org/sliding-sync:v0.99.12",
				ExposedPorts: []string{ssExposedPort},
				Env: map[string]string{
					"SYNCV3_SECRET":   "secret",
					"SYNCV3_BINDADDR": ":6789",
					"SYNCV3_SERVER":   "http://hs1:8008",
					"SYNCV3_DB":       "user=postgres dbname=syncv3 sslmode=disable password=postgres host=postgres",
				},
				WaitingFor: wait.ForLog("listening on"),
				Networks:   []string{networkName},
				NetworkAliases: map[string][]string{
					networkName: {"ssproxy"},
				},
			},
			Started: true,
		})
	must.NotError(t, "failed to start sliding sync container", err)

	ssURL := externalURL(t, ssContainer, ssExposedPort)
	csapi1 := deployment.UnauthenticatedClient(t, "hs1")
	csapi2 := deployment.UnauthenticatedClient(t, "hs2")

	// log for debugging purposes
	t.Logf("SlidingSyncDeployment created (network=%s):", networkName)
	t.Logf("  NAME          INT          EXT")
	t.Logf("  sliding sync: ssproxy      %s", ssURL)
	t.Logf("  synapse:      hs1          %s", csapi1.BaseURL)
	t.Logf("  synapse:      hs2          %s", csapi2.BaseURL)
	t.Logf("  postgres:     postgres")
	t.Logf("  mitmproxy:    mitmproxy hs1=%s hs2=%s controller=%s", rpHS1URL, rpHS2URL, controllerURL)
	var cmd *exec.Cmd
	if shouldTCPDump {
		t.Log("Running tcpdump...")
		su, _ := url.Parse(ssURL)
		cu1, _ := url.Parse(csapi1.BaseURL)
		cu2, _ := url.Parse(csapi2.BaseURL)
		filter := fmt.Sprintf("tcp port %s or port %s or port %s", su.Port(), cu1.Port(), cu2.Port())
		cmd = exec.Command("tcpdump", "-i", "any", "-s", "0", filter, "-w", "test.pcap")
		t.Log(cmd.String())
		if err := cmd.Start(); err != nil {
			t.Fatalf("tcpdump failed: %v", err)
		}
		// TODO needs sudo
		t.Logf("Started tcpdumping: PID %d", cmd.Process.Pid)
	}
	proxyURL, err := url.Parse(controllerURL)
	must.NotError(t, "failed to parse controller URL", err)
	t.Logf("mitm proxy url => %s", proxyURL.String())
	return &SlidingSyncDeployment{
		Deployment:     deployment,
		slidingSync:    ssContainer,
		postgres:       postgresContainer,
		reverseProxy:   mitmproxyContainer,
		slidingSyncURL: ssURL,
		tcpdump:        cmd,
		mitmClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		},
		proxyURLToHS: map[string]string{
			"hs1": rpHS1URL,
			"hs2": rpHS2URL,
		},
	}
}

func externalURL(t *testing.T, c testcontainers.Container, exposedPort string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	host, err := c.Host(ctx)
	must.NotError(t, "failed to get host", err)
	mappedPort, err := c.MappedPort(ctx, nat.Port(exposedPort))
	must.NotError(t, "failed to get mapped port", err)
	return fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
}

func writeContainerLogs(container testcontainers.Container, filename string) error {
	w, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("os.Create: %s", err)
	}
	reader, err := container.Logs(context.Background())
	if err != nil {
		return fmt.Errorf("container.Logs: %s", err)
	}
	_, err = io.Copy(w, reader)
	if err != nil {
		return fmt.Errorf("io.Copy: %s", err)
	}
	return nil
}
