package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config for the colors used in the tool
const (
	InfoColor        = "\033[1;34m%s\033[0m"
	NoticeColor      = "\033[1;36m%s\033[0m"
	WarningColor     = "\033[1;33m%s\033[0m"
	ErrorColor       = "\033[1;31m%s\033[0m"
	DebugColor       = "\033[0;36m%s\033[0m"
	CadenaSeparadora = "--------------------------------------------------------------------------------------------------------------------------\n"
)

// Config has been created
type Config struct {
	Insecure       bool `yaml:"insecure"`
	TimeoutRequest int  `yaml:"timeout_seconds"`
	Verbose        bool `yaml:"verbose"`
	Checks         []struct {
		Number       string  `yaml:"number"`
		URL          string  `yaml:"url"`
		StatusCode   *int    `yaml:"status_code"`
		Match        *string `yaml:"match"`
		ResponseTime *int    `yaml:"response_time"`
		TCP          string  `yaml:"tcp"`
		Port         *int    `yaml:"port"`
		Payload      string  `yaml:"payload"`
		Verbo        *string `yaml:"verbo"`
	} `yaml:"checks"`
}

// Config has been created
type CheckOutput struct {
	Number   string `json:"number"`
	Resource string `json:"resource"`
	Status   string `json:"available"`
	Elapsed  string `json:"elapsed"`
}

type JsonOutput struct {
	Results []CheckOutput `json:"checks"`
}

func addEntry(results []CheckOutput, url string, active bool, elapsed time.Duration, number string) []CheckOutput {
	check := &CheckOutput{
		Number:   number,
		Resource: url,
		Status:   strconv.FormatBool(!active),
		Elapsed:  elapsed.String(),
	}
	results = append(results, *check)
	return results
}

func main() {

	filenamePtr := flag.String("file", "monitor.yml", "Monitoring file")
	flag.Parse()

	hostUnreachable := false
	file, err := os.Open(*filenamePtr)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)

	y := Config{}

	err = yaml.Unmarshal([]byte(data), &y)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	results := &JsonOutput{}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: y.Insecure}

	for index, plugin := range y.Checks {
		_ = index
		tmpString := ""

		start := time.Now()

		if strings.Contains(plugin.Number, ".0") {
			fmt.Printf(InfoColor, CadenaSeparadora)
		}

		if strings.Contains(plugin.URL, "http") {
			jsonStr := []byte(plugin.Payload)
			req, err := http.NewRequest(*plugin.Verbo, plugin.URL, bytes.NewBuffer(jsonStr))
			if *plugin.Verbo == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}

			client := &http.Client{Timeout: time.Duration(y.TimeoutRequest) * time.Second}
			resp, err := client.Do(req)
			elapsed := time.Since(start)

			// if we fail connecting to the host
			if err != nil {
				tmpString = plugin.Number + " [NOK] " + plugin.URL + "\n"
				fmt.Printf(ErrorColor, tmpString)
				hostUnreachable = true

				results.Results = addEntry(results.Results, plugin.URL, hostUnreachable, elapsed, plugin.Number)
				continue
			}

			content, err := io.ReadAll(resp.Body)
			if y.Verbose {
				tmpString = plugin.Number + " URL : " + plugin.URL + "\n"
				// tmpString += "Status Code : " + string(resp.StatusCode) + "\n"
				tmpString += "Body : " + string(content)
				if len(tmpString) > 1000 {
					fmt.Println(tmpString[0:1000])
				} else {
					fmt.Println(tmpString[0:len(tmpString)])
				}
			}

			// if the status code does not correspond
			if plugin.StatusCode != nil && *plugin.StatusCode != resp.StatusCode {
				tmpString = plugin.Number + " [NOK] " + plugin.URL + "\n"
				fmt.Printf(ErrorColor, tmpString)
				hostUnreachable = true

				results.Results = addEntry(results.Results, plugin.URL, hostUnreachable, elapsed, plugin.Number)
				continue
			}

			// if your search string does not appear in the response body
			if plugin.Match != nil && !strings.Contains(string(content), *plugin.Match) {
				tmpString = plugin.Number + " [NOK] " + plugin.URL + "\n"
				fmt.Printf(ErrorColor, tmpString)
				hostUnreachable = true

				results.Results = addEntry(results.Results, plugin.URL, hostUnreachable, elapsed, plugin.Number)
				continue
			}

			// if http response takes more time than expected
			if plugin.ResponseTime != nil {
				responseTimeDuration := time.Duration(*plugin.ResponseTime) * time.Millisecond
				if responseTimeDuration-elapsed < 0 {
					responseTime := strconv.Itoa(*plugin.ResponseTime)
					tmpString = plugin.Number + " [NOK]  " + plugin.URL + ", Tiempo transcurrido: " + elapsed.String() + " en lugar de " + responseTime + "\n"
					fmt.Printf(ErrorColor, tmpString)
					hostUnreachable = true

					results.Results = addEntry(results.Results, plugin.URL, hostUnreachable, elapsed, plugin.Number)
					continue
				}
			}

			tmpString = plugin.Number + " [OK] " + plugin.URL + "\n"
			fmt.Printf(NoticeColor, tmpString)
			results.Results = addEntry(results.Results, plugin.URL, true, elapsed, plugin.Number)
		} else if plugin.TCP != "" {
			servAddr := plugin.TCP + ":" + strconv.Itoa(*plugin.Port)
			tcpAddr, err := net.ResolveTCPAddr("tcp", servAddr)
			conn, err := net.DialTCP("tcp", nil, tcpAddr)
			_ = conn
			// fmt.Println("Foobar?")
			elapsed := time.Since(start)
			if err != nil { // error on tcp connect
				hostUnreachable = true
				tmpString = plugin.Number + " [NOK] TCP:" + servAddr + "\n"
				fmt.Printf(ErrorColor, tmpString)
				results.Results = addEntry(results.Results, servAddr, hostUnreachable, elapsed, plugin.Number)
				continue
			} else if plugin.ResponseTime != nil { // error on connection
				responseTimeDuration := time.Duration(*plugin.ResponseTime) * time.Millisecond
				if responseTimeDuration-elapsed < 0 {
					responseTime := strconv.Itoa(*plugin.ResponseTime)
					tmpString = plugin.Number + " [NOK] TCP:" + servAddr + ", Tiempo transcurrido: " + elapsed.String() + " en lugar de " + responseTime + "\n"
					fmt.Printf(ErrorColor, tmpString)
					hostUnreachable = true
					results.Results = addEntry(results.Results, plugin.URL, hostUnreachable, elapsed, plugin.Number)
					continue
				}
			}
			tmpString = plugin.Number + " [OK] TCP:" + servAddr + "\n"
			fmt.Printf(NoticeColor, tmpString)
			results.Results = addEntry(results.Results, servAddr, true, elapsed, plugin.Number)
		}
	}

	jsonFile, _ := json.MarshalIndent(results, "", " ")
	_ = os.WriteFile("output.json", jsonFile, 0644)

	// if any host is unreachable, exit(1) to fail execution
	if hostUnreachable {
		os.Exit(1)
	}
	os.Exit(0)
}
