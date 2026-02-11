package service

import (
	"bytes"
	"regexp"

	"gopkg.in/yaml.v3"
)

// GenerateConfig generates the complete Stash YAML configuration
func GenerateConfig(proxies []ProxyNode) ([]byte, error) {
	var proxyNames []string
	for _, p := range proxies {
		if name, ok := p["name"].(string); ok {
			proxyNames = append(proxyNames, name)
		}
	}

	regionGroups := make(map[string][]string)
	regions := []string{"HK", "TW", "JP", "SG", "US", "KR"}
	for _, r := range regions {
		regionGroups[r] = []string{}
	}

	for _, name := range proxyNames {
		startRegions := classifyProxyName(name)
		for region, matched := range startRegions {
			if matched {
				regionGroups[region] = append(regionGroups[region], name)
			}
		}
	}

	config := map[string]interface{}{
		// ===== General Settings =====
		"mixed-port":          7890,
		"allow-lan":           true,
		"bind-address":        "*",
		"mode":                "rule",
		"log-level":           "info",
		"ipv6":                false,
		"external-controller": "0.0.0.0:9090",
		// ===== DNS =====
		"dns": map[string]interface{}{
			"enable":             true,
			"ipv6":               false,
			"listen":             "0.0.0.0:53",
			"default-nameserver": []string{"223.5.5.5", "119.29.29.29"},
			"enhanced-mode":      "fake-ip",
			"fake-ip-range":      "198.18.0.1/16",
			"fake-ip-filter": []string{
				"*.lan", "*.local", "*.crashlytics.com", "localhost.ptlogin2.qq.com",
				"+.srv.nintendo.net", "+.stun.playstation.net", "xbox.*.microsoft.com",
				"+.xboxlive.com", "+.msftconnecttest.com", "+.msftncsi.com",
			},
			"nameserver": []string{
				"https://doh.pub/dns-query",
				"https://dns.alidns.com/dns-query",
			},
			"fallback": []string{
				"https://1.1.1.1/dns-query",
				"https://dns.google/dns-query",
			},
			"fallback-filter": map[string]interface{}{
				"geoip":      true,
				"geoip-code": "CN",
				"ipcidr":     []string{"240.0.0.0/4", "0.0.0.0/32"},
			},
		},
		// ===== Proxies =====
		"proxies": proxies,
		// ===== Proxy Groups =====
		"proxy-groups": buildProxyGroups(proxyNames, regionGroups),
		// ===== Rules =====
		"rules": buildRules(),
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(config)
	return buf.Bytes(), err
}

func classifyProxyName(name string) map[string]bool {
	// Compile regex once in init() would be better but keeping it simple here
	// to match the structure.
	match := func(pattern string) bool {
		re := regexp.MustCompile(pattern)
		return re.MatchString(name)
	}

	return map[string]bool{
		"HK": match(`(?i)(?:🇭🇰|香港|HK|Hong\s*Kong)`),
		"TW": match(`(?i)(?:🇹🇼|台湾|TW|Taiwan)`),
		"JP": match(`(?i)(?:🇯🇵|日本|JP|Japan)`),
		"SG": match(`(?i)(?:🇸🇬|新加坡|SG|Singapore)`),
		"US": match(`(?i)(?:🇺🇸|美国|US|United\s*States|America)`),
		"KR": match(`(?i)(?:🇰🇷|韩国|KR|Korea)`),
	}
}

func buildProxyGroups(proxyNames []string, regionGroups map[string][]string) []map[string]interface{} {
	var groups []map[string]interface{}

	// Node Select
	groups = append(groups, map[string]interface{}{
		"name": "节点选择",
		"type": "select",
		"proxies": append([]string{
			"自动选择", "香港节点", "台湾节点", "日本节点",
			"新加坡节点", "美国节点", "韩国节点", "DIRECT",
		}, proxyNames...),
	})

	// Auto Select
	groups = append(groups, map[string]interface{}{
		"name":      "自动选择",
		"type":      "url-test",
		"proxies":   proxyNames,
		"url":       "http://www.gstatic.com/generate_204",
		"interval":  300,
		"tolerance": 50,
	})

	// Region Groups
	regionConfigs := []struct {
		Name string
		Key  string
	}{
		{"香港节点", "HK"},
		{"台湾节点", "TW"},
		{"日本节点", "JP"},
		{"新加坡节点", "SG"},
		{"美国节点", "US"},
		{"韩国节点", "KR"},
	}

	for _, rc := range regionConfigs {
		members := regionGroups[rc.Key]
		if len(members) > 0 {
			groups = append(groups, map[string]interface{}{
				"name":      rc.Name,
				"type":      "url-test",
				"proxies":   members,
				"url":       "http://www.gstatic.com/generate_204",
				"interval":  300,
				"tolerance": 50,
			})
		} else {
			// Fallback if no nodes for region
			proxies := proxyNames
			if len(proxies) == 0 {
				proxies = []string{"DIRECT"}
			}
			groups = append(groups, map[string]interface{}{
				"name":    rc.Name,
				"type":    "select",
				"proxies": proxies,
			})
		}
	}

	// Media
	groups = append(groups, map[string]interface{}{
		"name": "流媒体",
		"type": "select",
		"proxies": []string{
			"节点选择", "香港节点", "台湾节点", "日本节点",
			"新加坡节点", "美国节点", "韩国节点", "DIRECT",
		},
	})

	// AI Services
	groups = append(groups, map[string]interface{}{
		"name": "AI 服务",
		"type": "select",
		"proxies": []string{
			"美国节点", "日本节点", "新加坡节点", "节点选择", "DIRECT",
		},
	})

	// Final Fallback
	groups = append(groups, map[string]interface{}{
		"name":    "漏网之鱼",
		"type":    "select",
		"proxies": []string{"节点选择", "DIRECT"},
	})

	return groups
}

func buildRules() []string {
	return []string{
		// ----- AdBlock -----
		"DOMAIN-SUFFIX,ads.mopub.com,REJECT",
		"DOMAIN-SUFFIX,analytics.google.com,REJECT",
		// ----- AI Services -----
		"DOMAIN-SUFFIX,openai.com,AI 服务",
		"DOMAIN-SUFFIX,anthropic.com,AI 服务",
		"DOMAIN-SUFFIX,claude.ai,AI 服务",
		"DOMAIN-SUFFIX,bard.google.com,AI 服务",
		"DOMAIN-SUFFIX,gemini.google.com,AI 服务",
		"DOMAIN-SUFFIX,chat.openai.com,AI 服务",
		"DOMAIN-SUFFIX,sora.com,AI 服务",
		"DOMAIN-SUFFIX,chatgpt.com,AI 服务",
		"DOMAIN-KEYWORD,openai,AI 服务",
		// ----- Media -----
		"DOMAIN-SUFFIX,netflix.com,流媒体",
		"DOMAIN-SUFFIX,netflix.net,流媒体",
		"DOMAIN-SUFFIX,nflxvideo.net,流媒体",
		"DOMAIN-SUFFIX,nflximg.net,流媒体",
		"DOMAIN-SUFFIX,nflxext.com,流媒体",
		"DOMAIN-SUFFIX,disneyplus.com,流媒体",
		"DOMAIN-SUFFIX,disney-plus.net,流媒体",
		"DOMAIN-SUFFIX,hulu.com,流媒体",
		"DOMAIN-SUFFIX,hbo.com,流媒体",
		"DOMAIN-SUFFIX,hbomax.com,流媒体",
		"DOMAIN-SUFFIX,youtube.com,流媒体",
		"DOMAIN-SUFFIX,googlevideo.com,流媒体",
		"DOMAIN-SUFFIX,ytimg.com,流媒体",
		"DOMAIN-SUFFIX,spotify.com,流媒体",
		"DOMAIN-SUFFIX,twitch.tv,流媒体",
		// ----- Common Foreign -> Node Select -----
		"DOMAIN-SUFFIX,google.com,节点选择",
		"DOMAIN-SUFFIX,google.com.hk,节点选择",
		"DOMAIN-SUFFIX,googleapis.com,节点选择",
		"DOMAIN-SUFFIX,googlesource.com,节点选择",
		"DOMAIN-SUFFIX,gstatic.com,节点选择",
		"DOMAIN-SUFFIX,gmail.com,节点选择",
		"DOMAIN-SUFFIX,github.com,节点选择",
		"DOMAIN-SUFFIX,githubusercontent.com,节点选择",
		"DOMAIN-SUFFIX,github.io,节点选择",
		"DOMAIN-SUFFIX,githubassets.com,节点选择",
		"DOMAIN-SUFFIX,twitter.com,节点选择",
		"DOMAIN-SUFFIX,x.com,节点选择",
		"DOMAIN-SUFFIX,twimg.com,节点选择",
		"DOMAIN-SUFFIX,t.co,节点选择",
		"DOMAIN-SUFFIX,facebook.com,节点选择",
		"DOMAIN-SUFFIX,instagram.com,节点选择",
		"DOMAIN-SUFFIX,whatsapp.com,节点选择",
		"DOMAIN-SUFFIX,telegram.org,节点选择",
		"DOMAIN-SUFFIX,t.me,节点选择",
		"DOMAIN-SUFFIX,telegra.ph,节点选择",
		"DOMAIN-SUFFIX,wikipedia.org,节点选择",
		"DOMAIN-SUFFIX,wikimedia.org,节点选择",
		"DOMAIN-SUFFIX,reddit.com,节点选择",
		"DOMAIN-SUFFIX,redd.it,节点选择",
		"DOMAIN-SUFFIX,redditstatic.com,节点选择",
		"DOMAIN-SUFFIX,medium.com,节点选择",
		"DOMAIN-SUFFIX,notion.so,节点选择",
		"DOMAIN-SUFFIX,notion.site,节点选择",
		"DOMAIN-SUFFIX,discord.com,节点选择",
		"DOMAIN-SUFFIX,discordapp.com,节点选择",
		"DOMAIN-SUFFIX,slack.com,节点选择",
		"DOMAIN-SUFFIX,amazonaws.com,节点选择",
		"DOMAIN-SUFFIX,cloudflare.com,节点选择",
		"DOMAIN-SUFFIX,apple.com,DIRECT",
		"DOMAIN-SUFFIX,icloud.com,DIRECT",
		"DOMAIN-SUFFIX,microsoft.com,节点选择",
		"DOMAIN-SUFFIX,live.com,节点选择",
		"DOMAIN-SUFFIX,docker.com,节点选择",
		"DOMAIN-SUFFIX,docker.io,节点选择",
		"DOMAIN-SUFFIX,v2ex.com,节点选择",
		"DOMAIN-SUFFIX,stackoverflow.com,节点选择",
		"DOMAIN-SUFFIX,stackexchange.com,节点选择",
		"DOMAIN-SUFFIX,grammarly.com,节点选择",
		// ----- Domestic Direct -----
		"DOMAIN-SUFFIX,cn,DIRECT",
		"DOMAIN-SUFFIX,baidu.com,DIRECT",
		"DOMAIN-SUFFIX,bdstatic.com,DIRECT",
		"DOMAIN-SUFFIX,bilibili.com,DIRECT",
		"DOMAIN-SUFFIX,bilivideo.com,DIRECT",
		"DOMAIN-SUFFIX,hdslb.com,DIRECT",
		"DOMAIN-SUFFIX,zhihu.com,DIRECT",
		"DOMAIN-SUFFIX,douyin.com,DIRECT",
		"DOMAIN-SUFFIX,tiktokv.com,DIRECT",
		"DOMAIN-SUFFIX,taobao.com,DIRECT",
		"DOMAIN-SUFFIX,tmall.com,DIRECT",
		"DOMAIN-SUFFIX,alipay.com,DIRECT",
		"DOMAIN-SUFFIX,jd.com,DIRECT",
		"DOMAIN-SUFFIX,qq.com,DIRECT",
		"DOMAIN-SUFFIX,wechat.com,DIRECT",
		"DOMAIN-SUFFIX,weixin.qq.com,DIRECT",
		"DOMAIN-SUFFIX,163.com,DIRECT",
		"DOMAIN-SUFFIX,126.com,DIRECT",
		"DOMAIN-SUFFIX,csdn.net,DIRECT",
		"DOMAIN-SUFFIX,jianshu.com,DIRECT",
		"DOMAIN-SUFFIX,aliyun.com,DIRECT",
		"DOMAIN-SUFFIX,aliyuncs.com,DIRECT",
		"DOMAIN-SUFFIX,tencentcloud.com,DIRECT",
		"DOMAIN-SUFFIX,myqcloud.com,DIRECT",
		"DOMAIN-SUFFIX,feishu.cn,DIRECT",
		"DOMAIN-SUFFIX,feishu.net,DIRECT",
		"DOMAIN-SUFFIX,dingtalk.com,DIRECT",
		"DOMAIN-SUFFIX,meituan.com,DIRECT",
		"DOMAIN-SUFFIX,dianping.com,DIRECT",
		"DOMAIN-SUFFIX,xiaomi.com,DIRECT",
		"DOMAIN-SUFFIX,huawei.com,DIRECT",
		"DOMAIN-SUFFIX,weibo.com,DIRECT",
		"DOMAIN-SUFFIX,sinaimg.cn,DIRECT",
		"DOMAIN-SUFFIX,douban.com,DIRECT",
		"DOMAIN-SUFFIX,ctrip.com,DIRECT",
		// ----- LAN -----
		"DOMAIN-SUFFIX,local,DIRECT",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,100.64.0.0/10,DIRECT,no-resolve",
		// ----- GeoIP Direct -----
		"GEOIP,CN,DIRECT",
		// ----- Fallback -----
		"MATCH,漏网之鱼",
	}
}
