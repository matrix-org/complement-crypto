package deploy

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/matrix-org/complement"
	"github.com/matrix-org/complement/must"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type SlidingSyncDeployment struct {
	complement.Deployment
	postgres       testcontainers.Container
	slidingSync    testcontainers.Container
	slidingSyncURL string
}

func (d *SlidingSyncDeployment) SlidingSyncURL(t *testing.T) string {
	t.Helper()
	if d.slidingSync == nil || d.slidingSyncURL == "" {
		t.Fatalf("SlidingSyncURL: not set")
		return ""
	}
	return d.slidingSyncURL
}

func (d *SlidingSyncDeployment) Teardown() {
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
}

func RunNewDeployment(t *testing.T) *SlidingSyncDeployment {
	// allow 30s for everything to deploy
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Deploy the homeserver using Complement
	deployment := complement.Deploy(t, 1)
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

	// Make a sliding sync proxy
	ssExposedPort := "6789/tcp"
	ssContainer, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "ghcr.io/matrix-org/sliding-sync:v0.99.11",
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
	csapi := deployment.UnauthenticatedClient(t, "hs1")

	// log for debugging purposes
	t.Logf("SlidingSyncDeployment created (network=%s):", networkName)
	t.Logf("  NAME          INT / EXT")
	t.Logf("  sliding sync: ssproxy / %s", ssURL)
	t.Logf("  synapse:      hs1 / %s", csapi.BaseURL)
	t.Logf("  postgres:     postgres")
	return &SlidingSyncDeployment{
		Deployment:     deployment,
		slidingSync:    ssContainer,
		postgres:       postgresContainer,
		slidingSyncURL: ssURL,
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
