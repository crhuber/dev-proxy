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
	"regexp"
	"strings"
)

var version = "0.0.1"

func showVersion(version string) {
	fmt.Printf("version: %s", version)
}

func isRoot() bool {
	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf("Unable to get current user: %s", err)
	}
	return currentUser.Username == "root"
}

func status() {
	// use regular expression to match IP addresses in the output
	localhostRe := regexp.MustCompile(`127\.0\.0\.\d+`)

	// check hostfile
	file, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("==> Hosts file:")
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
	fmt.Println("\n==> Loopback interface lo0 addresses:")
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
	fmt.Println("\n==> Port forwarding rules:")
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

func hostEntryExists(file *os.File, ip string, host *string) bool {
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == fmt.Sprintf("%s\t%s", ip, *host) {
			fmt.Println("hostfile entry active")
			return true
		}
	}
	return false
}

func appendHostEntry(virtualIp string, host *string) {
	// Read host file
	file, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	if hostEntryExists(file, virtualIp, host) {
		return
	}

	hostEntry := fmt.Sprintf("\n%s\t%s", virtualIp, *host)
	_, err = file.WriteString(hostEntry)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		log.Fatal(err)
	}
	fmt.Printf("==> Hostfile updated: %s => %s\n", *host, virtualIp)
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
		return "", fmt.Errorf("No available IP address found")
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
		fmt.Printf("Removing alias: %s\n", alias)
		cmd := exec.Command("ifconfig", "lo0", "-alias", alias)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Error removing alias %v: %v", alias, err)
		}
	}

	return nil
}

func activate(host *string, port *int) {
	fmt.Println("Activating dev-proxy...")
	fmt.Println("")

	// get next available virtual ip
	virtualIp, err := getNextAvailableIP()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("==> Setting up virutal ip:", virtualIp)

	// Create an alias for virtualIP to point to loopback:
	_, err = exec.Command("ifconfig", "lo0", "alias", virtualIp).Output()
	if err != nil {
		fmt.Println("Error creating alias for loopback interface")
		log.Fatal(err.Error())
	}

	// Create a port forwarding rule to forward traffic destined for virtualIp:80 to be redirected to local application port
	// default port forward rules
	// nat-anchor "com.apple/*" all
	// rdr-anchor "com.apple/*" all
	redirectStr := fmt.Sprintf("rdr pass inet proto tcp from any to %s port 80 -> 127.0.0.1 port %d", virtualIp, *port)
	redirectCmd := exec.Command("echo", redirectStr)
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
	fmt.Println("==> Port forwarding: configured")

	// update hostfile
	appendHostEntry(virtualIp, host)

	fmt.Println("==> Dev proxy: ready!")

	fmt.Printf("\n[%s] => %s:80 => 127.0.0.1:%d \n", *host, virtualIp, *port)
}

// Show help menu
func showHelp() {
	fmt.Println("Usage: dev-proxy [activate|status|reset|version]")
	fmt.Println("Flags for 'activate' command:")
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
	case "activate":
		if !isRoot() {
			log.Fatal("dev-proxy activate needs to be run as sudo")
		}
		activateCmd := flag.NewFlagSet("activate", flag.ExitOnError)
		host := activateCmd.String("host", "dev.internal", "hostname that will resolve to a virtual ip")
		port := activateCmd.Int("port", 8080, "local port to proxy to")
		err := activateCmd.Parse(os.Args[2:])
		if err != nil {
			fmt.Println(err)
		}
		activate(host, port)

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
