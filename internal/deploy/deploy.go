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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement-crypto/internal/api"
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
	ControllerURL  string
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

func (d *SlidingSyncDeployment) WithSniffedEndpoint(t *testing.T, partialPath string, onSniff func(CallbackData), inner func()) {
	t.Helper()
	callbackURL, closeCallbackServer := NewCallbackServer(t, d, onSniff)
	defer closeCallbackServer()
	d.WithMITMOptions(t, map[string]interface{}{
		"callback": map[string]interface{}{
			"callback_url": callbackURL,
			// the filter is a python regexp
			// "Regexes are Python-style" - https://docs.mitmproxy.org/stable/concepts-filters/
			// re.escape() escapes very little:
			// "Changed in version 3.7: Only characters that can have special meaning in a regular expression are escaped.
			// As a result, '!', '"', '%', "'", ',', '/', ':', ';', '<', '=', '>', '@', and "`" are no longer escaped."
			// https://docs.python.org/3/library/re.html#re.escape
			//
			// The majority of HTTP paths are just /foo/bar with % for path-encoding e.g @foo:bar=>%40foo%3Abar,
			// so on balance we can probably just use the path directly.
			"filter": "~u .*" + partialPath + ".*",
		},
	}, func() {
		inner()
	})
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
	t.Logf("lockOptions: %v", string(jsonBody))
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
	t.Logf("unlockOptions")
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
	if writeLogs {
		containers := map[string]testcontainers.Container{
			"container-sliding-sync.log": d.slidingSync,
			"container-mitmproxy.log":    d.reverseProxy,
		}
		for filename, c := range containers {
			if c == nil {
				continue
			}
			logs, err := c.Logs(context.Background())
			if err != nil {
				log.Printf("failed to get logs for file %s: %s", filename, err)
				continue
			}
			err = writeContainerLogs(logs, filename)
			if err != nil {
				log.Printf("failed to write logs to %s: %s", filename, err)
			}
		}
		// and HSes..
		dockerClient, err := testcontainers.NewDockerClientWithOpts(context.Background())
		if err != nil {
			log.Printf("failed to write HS container logs, failed to make docker client: %s", err)
		} else {
			filenameToContainerID := map[string]string{
				"container-hs1.log": d.Deployment.ContainerID(&api.MockT{}, "hs1"),
				"container-hs2.log": d.Deployment.ContainerID(&api.MockT{}, "hs2"),
			}
			for filename, containerID := range filenameToContainerID {
				logs, err := dockerClient.ContainerLogs(context.Background(), containerID, types.ContainerLogsOptions{
					ShowStdout: true,
					ShowStderr: true,
					Follow:     false,
				})
				if err != nil {
					log.Printf("failed to get logs for container %s: %s", containerID, err)
					continue
				}
				err = writeContainerLogs(logs, filename)
				if err != nil {
					log.Printf("failed to write logs to %s: %s", filename, err)
				}
			}
		}
	}

	if d.slidingSync != nil {
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
		if err := d.reverseProxy.Terminate(context.Background()); err != nil {
			log.Fatalf("failed to stop reverse proxy: %s", err)
		}
	}
	if d.tcpdump != nil {
		fmt.Println("Sent SIGINT to tcpdump, waiting for it to exit, err=", d.tcpdump.Process.Signal(os.Interrupt))
		fmt.Println("tcpdump finished, err=", d.tcpdump.Wait())
	}
}

func RunNewDeployment(t *testing.T, mitmProxyAddonsDir string, shouldTCPDump bool) *SlidingSyncDeployment {
	// allow 30s for everything to deploy
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Deploy the homeserver using Complement
	deployment := complement.Deploy(t, 2)
	networkName := deployment.Network()

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

	// Make the mitmproxy and hardcode CONTAINER PORTS for hs1/hs2. HOST PORTS are still dynamically allocated.
	// By running this container on the same network as the homeservers, we can leverage DNS hence hs1/hs2 URLs.
	// We also need to preload addons into the proxy, so we bind mount the addons directory. This also allows
	// test authors to easily add custom addons.
	hs1ExposedPort := "3000/tcp"
	hs2ExposedPort := "3001/tcp"
	ssRevProxyExposedPort := "3002/tcp"
	controllerExposedPort := "8080/tcp" // default mitmproxy uses
	mitmContainerReq := testcontainers.ContainerRequest{
		Image:        "mitmproxy/mitmproxy:10.1.5",
		ExposedPorts: []string{hs1ExposedPort, hs2ExposedPort, controllerExposedPort, ssRevProxyExposedPort},
		Env:          map[string]string{},
		Cmd: []string{
			"mitmdump",
			"--mode", "reverse:http://hs1:8008@3000",
			"--mode", "reverse:http://hs2:8008@3001",
			"--mode", "reverse:http://ssproxy:6789@3002",
			"--mode", "regular",
		},
		Networks: []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"mitmproxy"},
		},
		HostConfigModifier: func(hc *container.HostConfig) {
			if runtime.GOOS == "linux" { // Specifically useful for GHA
				// Ensure that the container can contact the host, so they can
				// interact with a complement-controlled test server.
				// Note: this feature of docker landed in Docker 20.10,
				// see https://github.com/moby/moby/pull/40007
				hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}
			}
		},
	}
	if mitmProxyAddonsDir != "" {
		mitmContainerReq.Mounts = testcontainers.Mounts(
			testcontainers.BindMount(mitmProxyAddonsDir, "/addons"),
		)
		mitmContainerReq.Cmd = append(mitmContainerReq.Cmd, "-s", "/addons/__init__.py")
		mitmContainerReq.WaitingFor = wait.ForLog("loading complement crypto addons")
	}
	mitmproxyContainer, err := testcontainers.GenericContainer(context.Background(), testcontainers.GenericContainerRequest{
		ContainerRequest: mitmContainerReq,
		Started:          true,
	})
	must.NotError(t, "failed to start reverse proxy container", err)
	rpHS1URL := externalURL(t, mitmproxyContainer, hs1ExposedPort)
	rpHS2URL := externalURL(t, mitmproxyContainer, hs2ExposedPort)
	rpSSURL := externalURL(t, mitmproxyContainer, ssRevProxyExposedPort)
	controllerURL := externalURL(t, mitmproxyContainer, controllerExposedPort)

	// Make a sliding sync proxy
	ssExposedPort := "6789/tcp"
	ssContainer, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "ghcr.io/matrix-org/sliding-sync:v0.99.14",
				ExposedPorts: []string{ssExposedPort},
				Env: map[string]string{
					"SYNCV3_SECRET":    "secret",
					"SYNCV3_BINDADDR":  ":6789",
					"SYNCV3_SERVER":    "http://hs1:8008",
					"SYNCV3_LOG_LEVEL": "trace",
					"SYNCV3_DB":        "user=postgres dbname=syncv3 sslmode=disable password=postgres host=postgres",
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
	t.Logf("  sliding sync: ssproxy      %s (rp=%s)", ssURL, rpSSURL)
	t.Logf("  synapse:      hs1          %s (rp=%s)", csapi1.BaseURL, rpHS1URL)
	t.Logf("  synapse:      hs2          %s (rp=%s)", csapi2.BaseURL, rpHS2URL)
	t.Logf("  postgres:     postgres")
	t.Logf("  mitmproxy:    mitmproxy    controller=%s", controllerURL)
	var cmd *exec.Cmd
	if shouldTCPDump {
		t.Log("Running tcpdump...")
		urlsToTCPDump := []string{
			ssURL, csapi1.BaseURL, csapi2.BaseURL, rpHS1URL, rpHS2URL, controllerURL,
		}
		tcpdumpFilter := []string{}
		for _, u := range urlsToTCPDump {
			parsedURL, _ := url.Parse(u)
			tcpdumpFilter = append(tcpdumpFilter, fmt.Sprintf("port %s", parsedURL.Port()))
		}
		filter := fmt.Sprintf("tcp " + strings.Join(tcpdumpFilter, " or "))
		cmd = exec.Command("tcpdump", "-i", "any", "-s", "0", filter, "-w", "test.pcap")
		t.Log(cmd.String())
		if err := cmd.Start(); err != nil {
			t.Fatalf("tcpdump failed: %v", err)
		}
		t.Logf("Started tcpdumping (requires sudo): PID %d", cmd.Process.Pid)
	}
	// without this, GHA will fail when trying to hit the controller with "Post "http://mitm.code/options/lock": EOF"
	// suspected IPv4 vs IPv6 problems in Docker as Flask is listening on v4/v6.
	controllerURL = strings.Replace(controllerURL, "localhost", "127.0.0.1", 1)
	proxyURL, err := url.Parse(controllerURL)
	must.NotError(t, "failed to parse controller URL", err)
	return &SlidingSyncDeployment{
		Deployment:     deployment,
		slidingSync:    ssContainer,
		postgres:       postgresContainer,
		reverseProxy:   mitmproxyContainer,
		slidingSyncURL: rpSSURL,
		tcpdump:        cmd,
		ControllerURL:  controllerURL,
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

func writeContainerLogs(readCloser io.ReadCloser, filename string) error {
	w, err := os.Create("./logs/" + filename)
	if err != nil {
		return fmt.Errorf("os.Create: %s", err)
	}
	_, err = io.Copy(w, readCloser)
	if err != nil {
		return fmt.Errorf("io.Copy: %s", err)
	}
	return nil
}
