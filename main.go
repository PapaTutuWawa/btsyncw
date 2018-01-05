package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"golang.org/x/net/context"
)

const (
	DockerImage    string = "sync"
	DockerImageTag string = "slim"
)

// The structure of the config file
type Config struct {
	// For Resilio directly
	Folders []string
	Storage string
	Ip      string
	Network string

	// For my docker image
	Uid int64
	Gid int64
}

// Checks if an Commandline Injection is attempted
// Returns true, if the string is suspicious. false otherwise
func detectCmdInjection(input string) bool {
	re := regexp.MustCompile("(\"|')")
	return re.FindStringIndex(input) != nil
}

// Construct an array of environment variables that the container needs
func buildEnvVars(c *Config) []string {
	return []string{
		"USERID=" + strconv.FormatInt(c.Uid, 10),
		"GROUPID=" + strconv.FormatInt(c.Gid, 10),
	}
}

// Constructs the NetworkingConfig based on the provided Config
func buildNetConfig(c *Config) network.NetworkingConfig {
	ret := network.NetworkingConfig{}

	// Apply the IP Flag *only* if we got both an IP and a Network
	if c.Ip != "" && c.Network != "" {
		ret.EndpointsConfig = map[string]*network.EndpointSettings{
			c.Network: &network.EndpointSettings{
				IPAddress: c.Ip,
			},
		}
	}

	return ret
}

// Constructs the HostConfig based on the provided Config and the mounts
func buildHostConfig(c *Config, mounts *[]mount.Mount) container.HostConfig {
	ret := container.HostConfig{
		Mounts:     *mounts,
		AutoRemove: true,
	}

	// Apply the NetworkMode Flag only, if we have a Network specified
	if c.Network != "" {
		ret.NetworkMode = container.NetworkMode(c.Network)
	}

	return ret
}

// Constructs the container's config based on the provided Config
func buildContainerConfig(c *Config) container.Config {
	return container.Config{
		Image: DockerImage + ":" + DockerImageTag,
		Env:   buildEnvVars(c),
	}
}

// Checks if the passed JSON is safe to work with, e.g. contains all needed fields.
// Returns true, if the JSON is okay. false otherwise.
func validateConfig(c *Config) error {
	// "Storage" is required
	if c.Storage == "" {
		return errors.New("'Storage' field is required")
	}
	if detectCmdInjection(c.Storage) {
		return errors.New("Possible Commandline Injection found in 'Storage'")
	}

	// If we have Ip, then we need Network too
	if c.Ip != "" && c.Network == "" {
		return errors.New("The field 'Ip' requires 'Network'")
	}
	if detectCmdInjection(c.Ip) {
		return errors.New("Possible Commandline Injection found in 'Ip'")
	}
	if detectCmdInjection(c.Network) {
		return errors.New("Possible Commandline Injection found in 'Network'")
	}

	// Are the folders suspicious
	for i := 0; i < len(c.Folders); i++ {
		if detectCmdInjection(c.Folders[i]) {
			return errors.New("Possible Commandline Injection found in 'Folders'")
		}
	}

	// Do we got an UID and GID?
	if c.Uid == 0 || c.Gid == 0 {
		return errors.New("Field the fields 'Uid' and 'Gid' are required")
	}

	return nil
}

func main() {
	// Check if we have enough arguments
	if len(os.Args[1:]) != 1 {
		fmt.Println("Usage: btsyncw <config>")
		os.Exit(1)
	}

	// Try to open the file
	file, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Printf("Failed to open file: %v\n", err)
		os.Exit(1)
	}

	// Try to read the file
	buf := make([]byte, 1024)
	_, err = file.Read(buf)
	if err != nil {
		fmt.Printf("Could not read the file: %v\n", err)
		os.Exit(1)
	}
	// The linebreaks appear to be \x00 (NULL), which json *does not* like
	for i := 0; i < len(buf); i++ {
		if buf[i] == '\x00' {
			buf[i] = ' '
		}
	}

	// Try to convert it to a Config Struct
	var c Config
	err = json.Unmarshal(buf, &c)
	if err != nil {
		fmt.Printf("Failed to parse JSON: %v\n", err)
		os.Exit(1)
	}

	// Validate our JSON
	if err = validateConfig(&c); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Turn the paths into VolumeMounts
	mounts := make([]mount.Mount, 0)
	for fi := 0; fi < len(c.Folders); fi++ {
		// Find out the dirname (Though there's got to be a better way...)
		splitPath := strings.Split(c.Folders[fi], "/")
		dirname := splitPath[len(splitPath)-1]

		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: c.Folders[fi],
			Target: "/mnt/folders/" + dirname,
		})
	}
	// We append the storage path to make our life easier
	mounts = append(mounts, mount.Mount{
		Type:   mount.TypeBind,
		Source: c.Storage,
		Target: "/mnt/config",
	})

	// Connect to the docker daemon
	ctx := context.Background()
	cli, err := client.NewEnvClient()
	if err != nil {
		fmt.Printf("Failed to connect to docker: %v\n", err)
		os.Exit(1)
	}

	// Create the neccessary configuration
	containerConfig := buildContainerConfig(&c)
	hostConfig := buildHostConfig(&c, &mounts)
	netConfig := buildNetConfig(&c)

	// Create the container
	resp, err := cli.ContainerCreate(ctx, &containerConfig, &hostConfig, &netConfig, "Sync")
	if err != nil {
		fmt.Printf("Failed to create container: %v\n", err)
		os.Exit(1)
	}

	// Start the container
	if err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		fmt.Printf("Failed to start container: %v\n", err)
		os.Exit(1)
	}

	// Well, we did it
	fmt.Println("Started the Resilio Container")
}
