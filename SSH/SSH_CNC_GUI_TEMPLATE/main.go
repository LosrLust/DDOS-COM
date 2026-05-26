package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/BurntSushi/toml"

	"SSH-CNC-GUI/core"
)

type Config struct {
	General struct {
		Name    string
		Version string
	}
	SSH struct {
		Host    string
		Port    int
		KeyFile string `toml:"key_file"`
	}
	Database struct {
		Path string
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.General.Name == "" {
		c.General.Name = "SSH-CNC-GUI"
	}
	if c.General.Version == "" {
		c.General.Version = "1.0.0"
	}
	if c.SSH.Port == 0 {
		c.SSH.Port = 2222
	}
	if c.SSH.KeyFile == "" {
		c.SSH.KeyFile = "host.key"
	}
	if c.Database.Path == "" {
		c.Database.Path = "cnc.db"
	}
	return &c, nil
}

func outboundIP(host string) string {
	if host != "0.0.0.0" && host != "" && host != "::" {
		return host
	}
	c, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer c.Close()
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
			return a.IP.String()
		}
	}
	return "127.0.0.1"
}

func main() {
	log.SetFlags(0)

	fmt.Println()
	fmt.Println("  ╔═══════════════════════════╗")
	fmt.Println("  ║    SSH-CNC-GUI v1.0       ║")
	fmt.Println("  ╚═══════════════════════════╝")
	fmt.Println()

	path := "config.toml"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	cfg, err := loadConfig(path)
	if err != nil {
		log.Fatalf("  [!] config: %s", err)
	}
	fmt.Printf("  [+] Config: %s\n", path)

	if err := core.ConnectDB(cfg.Database.Path); err != nil {
		log.Fatalf("  [!] database: %s", err)
	}
	defer core.CloseDB()
	fmt.Printf("  [+] Database: %s\n", cfg.Database.Path)

	fmt.Printf("  [+] SSH: %s:%d\n\n", outboundIP(cfg.SSH.Host), cfg.SSH.Port)
	fmt.Println("  Ready. Waiting for connections...")
	fmt.Println()

	if err := core.StartSSH(cfg.SSH.Host, cfg.SSH.Port, cfg.SSH.KeyFile); err != nil {
		log.Fatalf("  [!] ssh: %s", err)
	}
}
