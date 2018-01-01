package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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

// Just for practice
type VolumeMount struct {
	From string
	To   string
	//	Readonly bool
}

// Checks if an Commandline Injection is attempted
// Returns true, if the string is suspicious. false otherwise
func detectCmdInjection(input string) bool {
	re := regexp.MustCompile("(\"|')")
	return re.FindStringIndex(input) != nil
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

// Builds the docker command from the config and the mounts
func buildCommand(c *Config, mounts []VolumeMount) *exec.Cmd {
	args := make([]string, 0)
	args = []string{"run", "--name", "Sync", "--rm"}

	// Append all volume mounts
	for i := 0; i < len(mounts); i++ {
		// TODO: Maybe use templates
		args = append(args, "--volume="+mounts[i].From+":"+mounts[i].To)
	}

	// Do we have an Ip
	if c.Network != "" {
		args = append(args, "--net="+c.Network)

		// Do we even specify an IP?
		if c.Ip != "" {
			args = append(args, "--ip="+c.Ip)
		}
	}

	// Append the UID and the GID as environment variables
	args = append(args, "--env=\"USERID="+strconv.FormatInt(c.Uid, 10)+"\"")
	args = append(args, "--env=\"GROUPID="+strconv.FormatInt(c.Gid, 10)+"\"")

	// Start the container detached
	args = append(args, "-d")

	// Append the image
	args = append(args, "sync:slim")

	return exec.Command("docker", args...)
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
	mounts := make([]VolumeMount, 0)
	for fi := 0; fi < len(c.Folders); fi++ {
		// Find out the dirname (Though there's got to be a better way...)
		splitPath := strings.Split(c.Folders[fi], "/")
		dirname := splitPath[len(splitPath)-1]

		mounts = append(mounts, VolumeMount{
			c.Folders[fi],
			"/mnt/folders/" + dirname,
		})
	}
	// We append the storage path to make our life easier
	mounts = append(mounts, VolumeMount{
		c.Storage,
		"/mnt/config",
	})

	// Build the command and execute it
	// fmt.Println(buildCommand(&c, mounts))
	cmd := buildCommand(&c, mounts)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("Failed to start the continer: %v\n", err)
		os.Exit(1)
	}

	// Well, we (maybe) did it
	fmt.Println("Started the Resilio Container")
}
