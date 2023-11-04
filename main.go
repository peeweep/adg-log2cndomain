package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/metacubex/geo/geoip"
	"github.com/metacubex/geo/geosite"
	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
	"net"
	"os"
	"strings"
)

// adguard home querylog.json
type AdgQueryJson struct {
	QH     string `json:"QH"`
	QT     string `json:"QT"`
	Answer string `json:"Answer"`
}

// config.yaml
type ConfigYaml struct {
	Adguardhome struct {
		QuerylogJson string `yaml:"querylogjson"`
	} `yaml:"adguardhome"`
	Geosite struct {
		File           string   `yaml:"file"`
		ExcludeCodes   []string `yaml:"excludeCodes"`
		ExcludeDomains []string `yaml:"excludeDomains"`
	} `yaml:"geosite"`
	Geoip struct {
		File         string   `yaml:"file"`
		IncludeCodes []string `yaml:"includeCodes"`
	} `yaml:"geoip"`
}

type Result struct{}

func main() {
	// 读取 YAML 文件
	yamlFile, err := os.ReadFile("config.yaml")
	if err != nil {
		fmt.Println("Error when loading", yamlFile)
		return
	}

	// 解析 YAML 文件
	var config ConfigYaml
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		fmt.Println(err)
		return
	}

	// geosite db
	geositeFilePath := config.Geosite.File
	geositeDb, err := geosite.FromFile(geositeFilePath)
	if err != nil {
		fmt.Println("Error when loading", geositeFilePath, "as a GeoSite database, skipped.")
		return
	}

	// geoip db
	geoipFilePath := config.Geoip.File
	geoipDb, err := geoip.FromFile(geoipFilePath)
	if err != nil {
		fmt.Println("Error when loading", geoipFilePath, "as a GeoIP database, skipped.")
		return
	}

	// 将JSON数据分割成多个条目
	jsonFile := config.Adguardhome.QuerylogJson
	fileContent, err := os.ReadFile(jsonFile)
	if err != nil {
		fmt.Println("Error when loading", jsonFile)
		return
	}
	entries := splitJSON(string(fileContent))

	msg := &dns.Msg{}
	var domains []string

	// 遍历每个条目并解析
	for i, entry := range entries {
		var adgQuery AdgQueryJson
		err := json.Unmarshal([]byte(entry), &adgQuery)
		if err != nil {
			fmt.Printf("解析第 %d 条JSON时出错: %s\n", i+1, err)
		} else {
			// base64 decode Answer
			decodedBytes, err := base64.StdEncoding.DecodeString(adgQuery.Answer)
			if err != nil {
				fmt.Println("解码时出错:", err)
				return
			}

			err = msg.Unpack([]byte(decodedBytes))
			if err != nil {
				fmt.Println("解析DNS消息时出错:", err)
				return
			}

			// check question is in geosite:cn
			newDomain, isNewDomain := checkGeosite(msg, geositeDb, domains,
				config.Geosite.ExcludeCodes, config.Geosite.ExcludeDomains)
			// update domains or skip
			if !isNewDomain {
				continue
			}

			// answer
			for _, answer := range msg.Answer {

				if answer.Header().Rrtype == dns.TypeA {
					ipAddr := strings.TrimPrefix(
						answer.String(),
						answer.Header().String())

					ipNetAddr := net.ParseIP(ipAddr)

					for _, code := range config.Geoip.IncludeCodes {
						if isGeoipCode(geoipDb, ipNetAddr, code) {
							domains = appendDomain(domains, newDomain)
						}
					}
				}
			}
		}
	}
	//fmt.Println(domains)
}

func appendDomain(domains []string, newDomain string) []string {
	for _, domain := range domains {
		if domain == newDomain {
			return domains
		}
	}
	//fmt.Printf("newDomain: %s\n", newDomain)
	fmt.Println(newDomain)

	return append(domains, newDomain)
}

func splitJSON(jsonData string) []string {
	// 将JSON数据按换行符分割成多个条目
	entries := []string{}
	start := 0
	for i := 0; i < len(jsonData); i++ {
		if jsonData[i] == '\n' {
			entries = append(entries, jsonData[start:i])
			start = i + 1
		}
	}
	// 添加最后一个条目（没有换行符）
	if start < len(jsonData) {
		entries = append(entries, jsonData[start:])
	}
	return entries
}

// domain is already in geosite:code
func isGeositeCode(db *geosite.Database, domain string, code string) bool {
	codes := db.LookupCodes(domain)
	for i := range codes {
		if codes[i] == code {
			return true
		}
	}
	return false
}

// parse geosite
func checkGeosite(msg *dns.Msg, db *geosite.Database, domains []string, excludeCodes []string, excludeDomains []string) (string, bool) {

	// question
	for _, question := range msg.Question {
		if question.Qtype == dns.TypeA {
			domainQuestionName := strings.TrimSuffix(question.Name, ".")
			//fmt.Println("check domain:  ", domainQuestionName)

			for _, code := range excludeCodes {
				if isGeositeCode(db, domainQuestionName, code) {
					return "", false
				}
			}
			for _, domain := range excludeDomains {
				if strings.HasSuffix(domainQuestionName, domain) {
					return "", false
				}
			}
			return domainQuestionName, true
		}
	}
	return "", false
}

// ip is already in geoip:code
func isGeoipCode(db *geoip.Database, ip net.IP, code string) bool {
	codes := db.LookupCode(ip)
	for i := range codes {
		if codes[i] == code {
			return true
		}
	}
	return false
}
