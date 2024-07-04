package deploy

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
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
	"github.com/matrix-org/complement/client"
	"github.com/matrix-org/complement/ct"
	"github.com/matrix-org/complement/helpers"
	"github.com/matrix-org/complement/must"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const mitmDumpFilePathOnContainer = "/tmp/mitm.dump"

type SlidingSyncDeployment struct {
	complement.Deployment
	extraContainers      map[string]testcontainers.Container
	mitmClient           *MITMClient
	ControllerURL        string
	dnsToReverseProxyURL map[string]string
	mu                   sync.RWMutex
	mitmDumpFile         string
}

// MITM returns a client capable of configuring man-in-the-middle operations such as
// snooping on CSAPI traffic and modifying responses.
func (d *SlidingSyncDeployment) MITM() *MITMClient {
	return d.mitmClient
}

func (d *SlidingSyncDeployment) UnauthenticatedClient(t ct.TestLike, serverName string) *client.CSAPI {
	return d.withReverseProxyURL(serverName, d.Deployment.UnauthenticatedClient(t, serverName))
}

func (d *SlidingSyncDeployment) Register(t ct.TestLike, hsName string, opts helpers.RegistrationOpts) *client.CSAPI {
	return d.withReverseProxyURL(hsName, d.Deployment.Register(t, hsName, opts))
}

func (d *SlidingSyncDeployment) Login(t ct.TestLike, hsName string, existing *client.CSAPI, opts helpers.LoginOpts) *client.CSAPI {
	return d.withReverseProxyURL(hsName, d.Deployment.Login(t, hsName, existing, opts))
}

func (d *SlidingSyncDeployment) AppServiceUser(t ct.TestLike, hsName, appServiceUserID string) *client.CSAPI {
	return d.withReverseProxyURL(hsName, d.Deployment.AppServiceUser(t, hsName, appServiceUserID))
}

func (d *SlidingSyncDeployment) SlidingSyncURLForHS(t ct.TestLike, hsName string) string {
	switch hsName {
	case "hs1":
		return d.dnsToReverseProxyURL["ssproxy1"]
	case "hs2":
		return d.dnsToReverseProxyURL["ssproxy2"]
	}
	ct.Fatalf(t, "SlidingSyncURLForHS: unknown hs name '%s'", hsName)
	return ""
}

// Replace the actual HS URL with a mitmproxy reverse proxy URL so we can sniff/intercept/modify traffic.
func (d *SlidingSyncDeployment) withReverseProxyURL(hsName string, c *client.CSAPI) *client.CSAPI {
	d.mu.RLock()
	defer d.mu.RUnlock()
	proxyURL := d.dnsToReverseProxyURL[hsName]
	c.BaseURL = proxyURL
	return c
}

func (d *SlidingSyncDeployment) writeMITMDump() {
	if d.mitmDumpFile == "" {
		return
	}
	log.Printf("dumping mitmdump to '%s'\n", d.mitmDumpFile)
	fileContents, err := d.extraContainers["mitmproxy"].CopyFileFromContainer(context.Background(), mitmDumpFilePathOnContainer)
	if err != nil {
		log.Printf("failed to copy mitmdump from container: %s", err)
		return
	}
	contents, err := io.ReadAll(fileContents)
	if err != nil {
		log.Printf("failed to read mitmdump: %s", err)
		return
	}
	if err = os.WriteFile(d.mitmDumpFile, contents, os.ModePerm); err != nil {
		log.Printf("failed to write mitmdump to %s: %s", d.mitmDumpFile, err)
		return
	}
}

func (d *SlidingSyncDeployment) Teardown() {
	d.writeMITMDump()
	for name, c := range d.extraContainers {
		filename := fmt.Sprintf("container-%s.log", name)
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

	for name, container := range d.extraContainers {
		if err := container.Terminate(context.Background()); err != nil {
			log.Fatalf("failed to stop %s: %s", name, err)
		}
	}
}

func RunNewDeployment(t *testing.T, mitmProxyAddonsDir string, mitmDumpFile string) *SlidingSyncDeployment {
	// allow time for everything to deploy
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Deploy the homeserver using Complement
	deployment := complement.Deploy(t, 2)
	networkName := deployment.Network()

	// rather than use POSTGRES_DB which only lets us make 1 db, inject some sql
	// to allow us to make 2 DBs, one for each SS instance on each HS.
	createdbFile := filepath.Join(os.TempDir(), "createdb.sql")
	err := os.WriteFile(createdbFile, []byte(`
	CREATE DATABASE syncv3_hs1;
	CREATE DATABASE syncv3_hs2;
	`), fs.ModePerm)
	if err != nil {
		ct.Fatalf(t, "failed to write createdb.sql: %s", err)
	}

	// Make a postgres container
	postgresContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:13-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "postgres",
				"POSTGRES_PASSWORD": "postgres",
			},
			Files: []testcontainers.ContainerFile{
				{
					HostFilePath:      createdbFile,
					ContainerFilePath: "/docker-entrypoint-initdb.d/create-dbs.sql",
					FileMode:          0o777,
				},
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
	ss1RevProxyExposedPort := "3002/tcp"
	ss2RevProxyExposedPort := "3003/tcp"
	controllerExposedPort := "8080/tcp" // default mitmproxy uses
	mitmContainerReq := testcontainers.ContainerRequest{
		Image:        "mitmproxy/mitmproxy:10.1.5",
		ExposedPorts: []string{hs1ExposedPort, hs2ExposedPort, controllerExposedPort, ss1RevProxyExposedPort, ss2RevProxyExposedPort},
		Env:          map[string]string{},
		Cmd: []string{
			"mitmdump",
			"--mode", "reverse:http://hs1:8008@3000",
			"--mode", "reverse:http://hs2:8008@3001",
			"--mode", "reverse:http://ssproxy1:6789@3002",
			"--mode", "reverse:http://ssproxy2:6789@3003",
			"--mode", "regular",
			"-w", mitmDumpFilePathOnContainer,
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
	mitmproxyContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: mitmContainerReq,
		Started:          true,
	})
	must.NotError(t, "failed to start reverse proxy container", err)
	rpHS1URL := externalURL(t, mitmproxyContainer, hs1ExposedPort)
	rpHS2URL := externalURL(t, mitmproxyContainer, hs2ExposedPort)
	rpSS1URL := externalURL(t, mitmproxyContainer, ss1RevProxyExposedPort)
	rpSS2URL := externalURL(t, mitmproxyContainer, ss2RevProxyExposedPort)
	controllerURL := externalURL(t, mitmproxyContainer, controllerExposedPort)

	// Make 2x sliding sync proxy
	ssExposedPort := "6789/tcp"
	ss1Container, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "ghcr.io/matrix-org/sliding-sync:v0.99.17",
				ExposedPorts: []string{ssExposedPort},
				Env: map[string]string{
					"SYNCV3_SECRET":    "secret",
					"SYNCV3_BINDADDR":  ":6789",
					"SYNCV3_SERVER":    "http://hs1:8008",
					"SYNCV3_LOG_LEVEL": "trace",
					"SYNCV3_DB":        "user=postgres dbname=syncv3_hs1 sslmode=disable password=postgres host=postgres",
				},
				WaitingFor: wait.ForLog("listening on"),
				Networks:   []string{networkName},
				NetworkAliases: map[string][]string{
					networkName: {"ssproxy1"},
				},
			},
			Started: true,
		})
	must.NotError(t, "failed to start sliding sync container", err)
	ss2Container, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "ghcr.io/matrix-org/sliding-sync:v0.99.17",
				ExposedPorts: []string{ssExposedPort},
				Env: map[string]string{
					"SYNCV3_SECRET":    "secret",
					"SYNCV3_BINDADDR":  ":6789",
					"SYNCV3_SERVER":    "http://hs2:8008",
					"SYNCV3_LOG_LEVEL": "trace",
					"SYNCV3_DB":        "user=postgres dbname=syncv3_hs2 sslmode=disable password=postgres host=postgres",
				},
				WaitingFor: wait.ForLog("listening on"),
				Networks:   []string{networkName},
				NetworkAliases: map[string][]string{
					networkName: {"ssproxy2"},
				},
			},
			Started: true,
		})
	must.NotError(t, "failed to start sliding sync container", err)

	ss1URL := externalURL(t, ss1Container, ssExposedPort)
	ss2URL := externalURL(t, ss2Container, ssExposedPort)
	csapi1 := deployment.UnauthenticatedClient(t, "hs1")
	csapi2 := deployment.UnauthenticatedClient(t, "hs2")

	// log for debugging purposes
	t.Logf("SlidingSyncDeployment created (network=%s):", networkName)
	t.Logf("  NAME          INT          EXT")
	t.Logf("  sliding sync: ssproxy1     %s (rp=%s)", ss1URL, rpSS1URL)
	t.Logf("  sliding sync: ssproxy2     %s (rp=%s)", ss2URL, rpSS2URL)
	t.Logf("  synapse:      hs1          %s (rp=%s)", csapi1.BaseURL, rpHS1URL)
	t.Logf("  synapse:      hs2          %s (rp=%s)", csapi2.BaseURL, rpHS2URL)
	t.Logf("  postgres:     postgres")
	t.Logf("  mitmproxy:    mitmproxy    controller=%s", controllerURL)
	// without this, GHA will fail when trying to hit the controller with "Post "http://mitm.code/options/lock": EOF"
	// suspected IPv4 vs IPv6 problems in Docker as Flask is listening on v4/v6.
	controllerURL = strings.Replace(controllerURL, "localhost", "127.0.0.1", 1)
	proxyURL, err := url.Parse(controllerURL)
	must.NotError(t, "failed to parse controller URL", err)
	return &SlidingSyncDeployment{
		Deployment: deployment,
		extraContainers: map[string]testcontainers.Container{
			"ssproxy1":  ss1Container,
			"ssproxy2":  ss2Container,
			"postgres":  postgresContainer,
			"mitmproxy": mitmproxyContainer,
		},
		ControllerURL: controllerURL,
		mitmClient:    NewMITMClient(proxyURL, deployment.GetConfig().HostnameRunningComplement),
		dnsToReverseProxyURL: map[string]string{
			"hs1":      rpHS1URL,
			"hs2":      rpHS2URL,
			"ssproxy1": rpSS1URL,
			"ssproxy2": rpSS2URL,
		},
		mitmDumpFile: mitmDumpFile,
	}
}

func externalURL(t *testing.T, c testcontainers.Container, exposedPort string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	host, err := c.Host(ctx)
	must.NotError(t, "failed to get host", err)
	if host == "localhost" {
		// always specify IPv4 addresses as otherwise you can get sporadic test failures
		// on IPv4/IPv6 enabled machines (e.g Github Actions) because:
		// - we do dynamic high numbered port allocation,
		// - allocated port namespaces are independent for v4 vs v6,
		// - meaning you can have 1 process bind to ::1:35678 and another process bind to 127.0.0.1:35678 RANDOMLY
		// - so if you get a request to http://localhost:35678...
		// - which process should be hit?
		// This manifests as test failures (typically endpoints that should work fine will 404 e.g HS requests hitting SS containers)
		// This can be fixed by replacing localhost with 127.0.01 in the request URL.
		host = "127.0.0.1"
	}
	mappedPort, err := c.MappedPort(ctx, nat.Port(exposedPort))
	must.NotError(t, "failed to get mapped port", err)
	return fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
}

func writeContainerLogs(readCloser io.ReadCloser, filename string) error {
	os.Mkdir("./logs", os.ModePerm) // ignore error, we don't care if it already exists
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
