package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/metacubex/geo/geosite"
	"github.com/miekg/dns"
	"os"
	"strings"
)

// adguard home querylog.json
type AdgQuery struct {
	QH     string `json:"QH"`
	QT     string `json:"QT"`
	Answer string `json:"Answer"`
}

type Result struct{}

func main() {

	geositeFilePath := "/home/peeweep/.geo/geosite.dat"
	db, err := geosite.FromFile(geositeFilePath)
	if err != nil {
		fmt.Println("Error when loading", geositeFilePath, "as a GeoSite database, skipped.")
		return
	}

	// 将JSON数据分割成多个条目
	jsonFile := "querylog.json"
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
		var adgQuery AdgQuery
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
			newDomains := checkGeosite(msg, db, domains)
			// update domains or skip
			if len(newDomains) == len(domains) {
				continue
			} else {
				domains = newDomains
			}

			// answer
			for _, answer := range msg.Answer {

				if answer.Header().Rrtype == dns.TypeA {
					ipAddr := strings.TrimPrefix(
						answer.String(),
						answer.Header().String())

					domainAnswerName := strings.TrimSuffix(answer.Header().Name, ".")

					fmt.Printf("Answer: %s %s\n", domainAnswerName, ipAddr)
				}
			}

		}
	}
	fmt.Println(domains)

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

// is already in geosite:cn
func isGeositeCN(db *geosite.Database, domain string) bool {
	codes := db.LookupCodes(domain)
	for i := range codes {
		if codes[i] == "cn" {
			return true
		}
	}
	return false
}

// parse geosite
func checkGeosite(msg *dns.Msg, db *geosite.Database, domains []string) []string {

	// question
	for _, question := range msg.Question {
		if question.Qtype == dns.TypeA {
			domainQuestionName := strings.TrimSuffix(question.Name, ".")
			fmt.Println("check domain:  ", domainQuestionName)
			//fmt.Printf("Question: %s %s\n", domainQuestionName, dns.Type(question.Qtype).String())

			if !isGeositeCN(db, domainQuestionName) {
				domains = append(domains, domainQuestionName)
				fmt.Println("add domain: ", domainQuestionName)
				return domains
			}
		}
	}
	return domains

}
