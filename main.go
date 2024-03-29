package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml"
)

// define contansts
const (
	configFileName = "config.toml"
	configDirName  = ".devproxy"
	virtualPort    = 80
	version        = "0.0.5"
	Reset          = "\033[0m"
	Red            = "\033[31m"
	Green          = "\033[32m"
	Yellow         = "\033[33m"
	Blue           = "\033[34m"
	Purple         = "\033[35m"
	Cyan           = "\033[36m"
	Gray           = "\033[37m"
	White          = "\033[97m"
)

func showVersion(version string) {
	fmt.Printf("version: %s \n", version)
}

func printLnColor(text string, colorChoice string) {
	switch colorChoice {
	case "red":
		fmt.Println(Red + text + Reset)
	case "green":
		fmt.Println(Green + text + Reset)
	case "yellow":
		fmt.Println(Yellow + text + Reset)
	case "blue":
		fmt.Println(Blue + text + Reset)
	default:
		fmt.Println(White + text + Reset)
	}
}

func isRoot() bool {
	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf("Unable to get current user: %s", err)
	}
	return currentUser.Username == "root"
}

func show() {
	config, err := readTomlConfig()
	if err != nil {
		log.Fatal(err.Error())

	}

	for _, key := range config.Keys() {
		port := config.Get(key + ".port")
		hostname := config.Get(key + ".hostname")
		virtualIp := config.Get(key + ".virtualIP")
		printLnColor(fmt.Sprintf("\n[%s]", hostname), "blue")
		fmt.Printf("localIp: 127.0.0.1:%d \n", port)
		fmt.Printf("virtualIp: %s:%d \n", virtualIp, virtualPort)
	}

}

func status() {
	// use regular expression to match IP addresses in the output
	localhostRe := regexp.MustCompile(`127\.0\.0\.\d+`)

	// check hostfile
	file, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	printLnColor("==> Hosts file:", "green")
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if localhostRe.MatchString(line) {
			fmt.Println(line)
		}
	}

	// check loopback
	// use regular expression to match IP addresses in the output
	inetRe := regexp.MustCompile(`inet\s+127\.0\.0\.\d+`)
	printLnColor("\n==> Loopback interface lo0 addresses:", "green")
	ifCmdOutput, err := exec.Command("ifconfig", "lo0").Output()
	if err != nil {
		log.Fatal(err)
	}
	matches2 := inetRe.FindAllString(string(ifCmdOutput), -1)
	if len(matches2) > 0 {
		for _, match := range matches2 {
			ip := strings.Split(match, " ")[1]
			fmt.Println(ip)
		}
	} else {
		fmt.Println("No 127.0.0.* addresses assigned to lo0")
	}

	// check pftctl command
	printLnColor("\n==> Port forwarding rules:", "green")
	pfCtlCmdOutput, err := exec.Command("pfctl", "-s", "nat").Output()
	if err != nil {
		log.Fatal(err)
	}

	lines := strings.Split(string(pfCtlCmdOutput), "\n")
	for _, line := range lines {
		if localhostRe.MatchString(line) {
			fmt.Println(line)
		}
	}

}

func hostEntryExists(file *os.File, ip string, host string) bool {
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == fmt.Sprintf("%s\t%s", ip, host) {
			fmt.Printf("entry `%s %s` already exists \n", ip, host)
			return true
		}
	}
	return false
}

func appendHostEntry(virtualIp string, host string) {
	// Read host file
	file, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	if hostEntryExists(file, virtualIp, host) {
		return
	}

	// write to file
	hostEntry := fmt.Sprintf("\n%s\t%s", virtualIp, host)
	_, err = file.WriteString(hostEntry)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		log.Fatal(err)
	}
	fmt.Printf("Added %s %s\n", host, virtualIp)
}

func getNextAvailableIP() (string, error) {
	// get addresses assigned to lo0 interface
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	var lo0Addrs []net.Addr
	for _, iface := range ifaces {
		if iface.Name == "lo0" {
			addrs, err := iface.Addrs()
			if err != nil {
				return "", err
			}
			lo0Addrs = addrs
			break
		}
	}

	// find the next available IPv4 address
	lastIP := net.ParseIP("127.0.0.1")
	for _, addr := range lo0Addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}

		if ip.To4() == nil {
			continue
		}

		if ip.To4()[0] == 127 && ip.To4()[1] == 0 && ip.To4()[2] == 0 && ip.To4()[3] > lastIP.To4()[3] {
			lastIP = ip
		}
	}

	// increment the last IP address and return it as a string
	if lastIP.To4()[3] == 255 {
		return "", fmt.Errorf("no available IP address found")
	}
	nextIP := net.IPv4(lastIP.To4()[0], lastIP.To4()[1], lastIP.To4()[2], lastIP.To4()[3]+1)
	return nextIP.String(), nil
}

func reset() {
	removeLo0Aliases()
}

func removeLo0Aliases() error {
	// check loopback
	// use regular expression to match IP addresses in the output
	inetRe := regexp.MustCompile(`inet\s+127\.0\.0\.\d+`)
	fmt.Println("\n==> Loopback interface lo0 addresses:")
	ifCmdOutput, err := exec.Command("ifconfig", "lo0").Output()
	if err != nil {
		log.Fatal(err)
	}
	matches2 := inetRe.FindAllString(string(ifCmdOutput), -1)
	aliases := []string{}
	if len(matches2) > 0 {
		for _, match := range matches2 {
			ip := strings.Split(match, " ")[1]
			if ip != "127.0.0.1" {
				aliases = append(aliases, ip)
			}
		}
	} else {
		fmt.Println("No 127.0.0.* addresses assigned to lo0")
	}

	// remove each alias using the ifconfig command
	for _, alias := range aliases {
		fmt.Printf("Removing alias: %s \n", alias)
		cmd := exec.Command("ifconfig", "lo0", "-alias", alias)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error removing alias %v: %v", alias, err)
		}
	}

	return nil
}

func writeTomlConfig(hostname string, port int) error {
	var usr *user.User
	var err error

	// get the config file from the current user that ran sudo
	usr, err = user.Lookup(os.ExpandEnv("$USER"))
	if err != nil {
		return fmt.Errorf("failed to get the sudo user: %w", err)
	}

	configDir := filepath.Join(usr.HomeDir, ".devproxy")
	configFile := filepath.Join(configDir, "config.toml")

	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	var config *toml.Tree
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		config, _ = toml.Load("")
	} else if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	} else {
		configBytes, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		config, err = toml.LoadBytes(configBytes)
		if err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	hostnamePrefix := strings.Split(hostname, ".")[0]

	// find the section index for the hostname
	sectionIndex := -1
	sections := config.Keys()
	for i, section := range sections {
		if section == hostnamePrefix {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		sectionIndex = len(sections)
	}

	// use the section index to find the next available ip
	// find the next available IPv4 address
	baseIP := net.ParseIP("127.0.0.1")

	// increment the last IP address and return it as a string
	if baseIP.To4()[3] == 255 {
		return fmt.Errorf("no available IP left")
	}
	nextIP := net.IPv4(baseIP.To4()[0], baseIP.To4()[1], baseIP.To4()[2], baseIP.To4()[3]+byte(sectionIndex)+1)

	config.Set(hostnamePrefix+".port", int64(port))
	config.Set(hostnamePrefix+".hostname", hostname)
	config.Set(hostnamePrefix+".virtualIP", nextIP.String())
	configString, err := config.ToTomlString()
	if err != nil {
		return fmt.Errorf("failed to convert config to string: %w", err)
	}

	err = os.WriteFile(configFile, []byte(configString), 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	printLnColor("==> Dev proxy: Config file updated!", "green")

	return nil
}

func readTomlConfig() (*toml.Tree, error) {
	var usr *user.User
	var err error

	// get the config file from the current user that ran sudo
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser != "" {
		usr, err = user.Lookup(sudoUser)
		if err != nil {
			return nil, fmt.Errorf("failed to get the sudo user: %w", err)
		}
	} else {
		usr, err = user.Current()
		if err != nil {
			return nil, fmt.Errorf("failed to get the current user: %w", err)
		}
	}

	configDir := filepath.Join(usr.HomeDir, ".devproxy")
	configFile := filepath.Join(configDir, "config.toml")
	fmt.Printf("Config: %s \n", configFile)

	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config, err := toml.LoadBytes(configBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

func up() {
	printLnColor("Activating dev-proxy...", "white")

	config, err := readTomlConfig()
	if err != nil {
		log.Fatal(err.Error())

	}
	// if there are no keys in the config file, exit
	if len(config.Keys()) == 0 {
		log.Fatal("==> Dev proxy: not configured. Run dev-proxy add --help for more info")
		return
	}

	redirectionRules := ""
	for _, key := range config.Keys() {
		port := config.Get(key + ".port")
		hostname := config.Get(key + ".hostname")
		virtualIp := config.Get(key + ".virtualIP")
		printLnColor(fmt.Sprintf("\n[%s]", hostname), "blue")
		printLnColor("==> Setting up virtual ip", "green")
		fmt.Printf("ip: %s \n", virtualIp)

		// Create an alias for virtualIP to point to loopback:
		_, err = exec.Command("ifconfig", "lo0", "alias", virtualIp.(string)).Output()
		if err != nil {
			fmt.Println("Error creating alias for loopback interface")
			log.Fatal(err.Error())
		}

		// update hostfile
		printLnColor(fmt.Sprintf("==> Updating hostfile: %s", hostname), "green")
		appendHostEntry(virtualIp.(string), hostname.(string))

		printLnColor("==> Configuring port fowarding", "green")
		fmt.Printf("%s => %s:%d => 127.0.0.1:%d \n", hostname, virtualIp, virtualPort, port)
		// Create a port forwarding rule to forward traffic destined for virtualIp:80 to be redirected to local application port
		// default port forward rules
		// nat-anchor "com.apple/*" all
		// rdr-anchor "com.apple/*" all
		redirectStr := fmt.Sprintf("rdr pass inet proto tcp from any to %s port %d -> 127.0.0.1 port %d\n", virtualIp, virtualPort, port)
		redirectionRules += redirectStr
	}

	// execute port forwarding rule
	printLnColor("==> Applying rules", "green")
	redirectCmd := exec.Command("echo", redirectionRules)
	pfCmd := exec.Command("pfctl", "-ef", "-")

	// Get the pipe of Stdout from eco command and assign it
	// to the Stdin of pfctl.
	pipe, err := redirectCmd.StdoutPipe()
	if err != nil {
		fmt.Println("Error running pfctl redirect command")
		log.Fatal(err)
	}
	pfCmd.Stdin = pipe

	// Start() echo command, so we don't block on it.
	err = redirectCmd.Start()
	if err != nil {
		fmt.Println("Error running pfctl redirect command")
		log.Fatal(err)
	}

	// Run Output() on pfctl to capture the output.
	_, _ = pfCmd.Output()
	// sometimes pfctl -ef - returns exitcode 1 even if theres no error
	// dont exit fatal here
	fmt.Println("port forwarding: configured")

	printLnColor("\ndev-proxy: running!", "white")

}

// Show help menu
func showHelp() {
	fmt.Println("Usage: dev-proxy [add|show|status|reset|up|version]")
	flag.PrintDefaults()
}

func main() {

	// Check if a sub-command was provided
	if len(os.Args) < 2 {
		// Show help menu
		showHelp()
		return
	}

	switch os.Args[1] {
	case "add":
		if isRoot() {
			log.Fatal("dev-proxy add should not be run as sudo")
		}
		addCmd := flag.NewFlagSet("activate", flag.ExitOnError)
		host := addCmd.String("host", "dev.internal", "hostname that will resolve to a virtual ip")
		port := addCmd.Int("port", 8080, "local port to proxy to")
		err := addCmd.Parse(os.Args[2:])
		if err != nil {
			fmt.Println(err)
		}
		err = writeTomlConfig(*host, *port)
		if err != nil {
			fmt.Println("Error:", err)
		}

	case "show":
		show()

	case "up":
		if !isRoot() {
			log.Fatal("dev-proxy up needs to be run as sudo")
		}
		up()

	case "status":
		if !isRoot() {
			log.Fatal("dev-proxy status needs to be run as sudo")
		}
		status()

	case "reset":
		if !isRoot() {
			log.Fatal("dev-proxy reset needs to be run as sudo")
		}
		reset()

	case "version":
		showVersion(version)

	default:
		showHelp()
	}
}
