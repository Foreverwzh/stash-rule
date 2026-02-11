package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"my-stash-rule/internal/model"
	"gopkg.in/yaml.v3"
)

var userAgent = "Stash/2.7.0 Clash/1.0"

// ProxyNode represents a proxy node configuration
type ProxyNode = model.ProxyNode

// FetchProxies concurrency fetches proxies from multiple URLs
func FetchProxies(urls []string) ([]ProxyNode, error) {
	var (
		allProxies []ProxyNode
		mu         sync.Mutex
		wg         sync.WaitGroup
	)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	for _, u := range urls {
		wg.Add(1)
		go func(targetURL string) {
			defer wg.Done()
			proxies, err := fetchSingleURL(client, targetURL)
			if err != nil {
				log.Printf("Failed to fetch from %s: %v", targetURL, err)
				return
			}
			if len(proxies) > 0 {
				parsedURL, _ := url.Parse(targetURL)
				host := ""
				if parsedURL != nil {
					host = parsedURL.Hostname()
				}
				log.Printf("Fetched %d proxies from %s", len(proxies), host)
				mu.Lock()
				allProxies = append(allProxies, proxies...)
				mu.Unlock()
			}
		}(u)
	}

	wg.Wait()
	return allProxies, nil
}

func fetchSingleURL(client *http.Client, url string) ([]ProxyNode, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseSubscription(string(body)), nil
}

func parseSubscription(content string) []ProxyNode {
	content = strings.TrimSpace(content)

	// Try parsing as YAML
	var yamlData struct {
		Proxies []ProxyNode `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(content), &yamlData); err == nil && len(yamlData.Proxies) > 0 {
		return yamlData.Proxies
	}

	// Try parsing as Base64 encoded list
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		// Try URL-safe base64 if standard fails, or handle padding
		if padding := len(content) % 4; padding != 0 {
			content += strings.Repeat("=", 4-padding)
		}
		decoded, err = base64.StdEncoding.DecodeString(content)
	}
	
	if err == nil {
		content = string(decoded)
	}
	
	// Either successfully decoded or treating original content as list
	var proxies []ProxyNode
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if proxy := parseURI(line); proxy != nil {
			proxies = append(proxies, proxy)
		}
	}

	return proxies
}

func parseURI(uri string) ProxyNode {
	if strings.HasPrefix(uri, "vmess://") {
		return parseVMess(uri)
	} else if strings.HasPrefix(uri, "trojan://") {
		return parseTrojan(uri)
	} else if strings.HasPrefix(uri, "ss://") {
		return parseSS(uri)
	} else if strings.HasPrefix(uri, "ssr://") {
		return parseSSR(uri)
	}
	return nil
}

func parseVMess(uri string) ProxyNode {
	raw := strings.TrimPrefix(uri, "vmess://")
	if padding := len(raw) % 4; padding != 0 {
		raw += strings.Repeat("=", 4-padding)
	}
	
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(decoded, &data); err != nil {
		return nil
	}

	proxy := ProxyNode{
		"name":    getString(data, "ps", "VMess节点"),
		"type":    "vmess",
		"server":  getString(data, "add", ""),
		"port":    getInt(data, "port", 443),
		"uuid":    getString(data, "id", ""),
		"alterId": getInt(data, "aid", 0),
		"cipher":  getString(data, "scy", "auto"),
		"udp":     true,
	}

	if getString(data, "tls", "") == "tls" {
		proxy["tls"] = true
		if sni := getString(data, "sni", ""); sni != "" {
			proxy["servername"] = sni
		}
	}

	net := getString(data, "net", "tcp")
	if net == "ws" {
		proxy["network"] = "ws"
		wsOpts := make(map[string]interface{})
		if path := getString(data, "path", ""); path != "" {
			wsOpts["path"] = path
		}
		if host := getString(data, "host", ""); host != "" {
			wsOpts["headers"] = map[string]string{"Host": host}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	} else if net == "grpc" {
		proxy["network"] = "grpc"
		if path := getString(data, "path", ""); path != "" {
			proxy["grpc-opts"] = map[string]string{"grpc-service-name": path}
		}
	}

	return proxy
}

func parseTrojan(uri string) ProxyNode {
	u, err := url.Parse(uri)
	if err != nil {
		return nil
	}

	query := u.Query()
	name := u.Fragment
	if name == "" {
		name = "Trojan节点"
	} else {
		name, _ = url.QueryUnescape(name)
	}

	port := 443
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}

	proxy := ProxyNode{
		"name":     name,
		"type":     "trojan",
		"server":   u.Hostname(),
		"port":     port,
		"password": u.User.Username(),
		"udp":      true,
	}

	if sni := query.Get("sni"); sni != "" {
		proxy["sni"] = sni
	}
	if query.Get("allowInsecure") == "1" {
		proxy["skip-cert-verify"] = true
	}

	if query.Get("type") == "ws" {
		proxy["network"] = "ws"
		wsOpts := make(map[string]interface{})
		if path := query.Get("path"); path != "" {
			wsOpts["path"] = path
		}
		if host := query.Get("host"); host != "" {
			wsOpts["headers"] = map[string]string{"Host": host}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	}

	return proxy
}

func parseSS(uri string) ProxyNode {
	raw := strings.TrimPrefix(uri, "ss://")
	
	name := "SS节点"
	if idx := strings.LastIndex(raw, "#"); idx != -1 {
		nameStr := raw[idx+1:]
		if unescaped, err := url.QueryUnescape(nameStr); err == nil {
			name = unescaped
		}
		raw = raw[:idx]
	}

	var method, password, server string
	var port int

	if strings.Contains(raw, "@") {
		parts := strings.SplitN(raw, "@", 2)
		userInfo := parts[0]
		serverInfo := parts[1]

		if padding := len(userInfo) % 4; padding != 0 {
			userInfo += strings.Repeat("=", 4-padding)
		}
		decoded, err := base64.StdEncoding.DecodeString(userInfo)
		if err == nil {
			userParts := strings.SplitN(string(decoded), ":", 2)
			if len(userParts) == 2 {
				method = userParts[0]
				password = userParts[1]
			}
		}
		
		serverParts := strings.SplitN(serverInfo, ":", 2)
		if len(serverParts) == 2 {
			server = serverParts[0]
			// Clear any query params from port
			portStr := strings.Split(strings.Split(serverParts[1], "?")[0], "/")[0]
			port, _ = strconv.Atoi(portStr)
		}
	} else {
		if padding := len(raw) % 4; padding != 0 {
			raw += strings.Repeat("=", 4-padding)
		}
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err == nil {
			str := string(decoded)
			if idx := strings.LastIndex(str, "@"); idx != -1 {
				methodPas := str[:idx]
				serverInfo := str[idx+1:]
				
				mpParts := strings.SplitN(methodPas, ":", 2)
				if len(mpParts) == 2 {
					method = mpParts[0]
					password = mpParts[1]
				}

				srvParts := strings.SplitN(serverInfo, ":", 2)
				if len(srvParts) == 2 {
					server = srvParts[0]
					port, _ = strconv.Atoi(srvParts[1])
				}
			}
		}
	}

	if server == "" || method == "" {
		return nil
	}

	return ProxyNode{
		"name":     name,
		"type":     "ss",
		"server":   server,
		"port":     port,
		"cipher":   method,
		"password": password,
		"udp":      true,
	}
}

func parseSSR(uri string) ProxyNode {
	raw := strings.TrimPrefix(uri, "ssr://")
	if padding := len(raw) % 4; padding != 0 {
		raw += strings.Repeat("=", 4-padding)
	}
	
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}

	// server:port:protocol:method:obfs:base64pass/?params
	mainPart := strings.Split(string(decoded), "/?")[0]
	parts := strings.Split(mainPart, ":")

	if len(parts) < 6 {
		return nil
	}

	server := parts[0]
	port, _ := strconv.Atoi(parts[1])
	protocol := parts[2]
	method := parts[3]
	obfs := parts[4]
	
	passB64 := parts[5]
	if padding := len(passB64) % 4; padding != 0 {
		passB64 += strings.Repeat("=", 4-padding)
	}
	password := ""
	if passBytes, err := base64.StdEncoding.DecodeString(passB64); err == nil {
		password = string(passBytes)
	}

	return ProxyNode{
		"name":     fmt.Sprintf("SSR-%s:%d", server, port),
		"type":     "ssr",
		"server":   server,
		"port":     port,
		"cipher":   method,
		"password": password,
		"protocol": protocol,
		"obfs":     obfs,
		"udp":      true,
	}
}

// Helpers

func getString(data map[string]interface{}, key, defaultValue string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultValue
}

func getInt(data map[string]interface{}, key string, defaultValue int) int {
	if v, ok := data[key]; ok {
		switch i := v.(type) {
		case float64:
			return int(i)
		case int:
			return i
		case string:
			if val, err := strconv.Atoi(i); err == nil {
				return val
			}
		}
	}
	return defaultValue
}
