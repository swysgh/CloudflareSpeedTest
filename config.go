package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/XIU2/CloudflareSpeedTest/cfdns"
	"github.com/XIU2/CloudflareSpeedTest/task"
	"github.com/XIU2/CloudflareSpeedTest/utils"
)

// cfOptions 是「测速完成后自动更新 Cloudflare DNS」的一组参数，命令行 flag 与配置文件共用
type cfOptions struct {
	zone    string
	record  string
	token   string
	count   int
	ttl     int
	proxied bool
}

var cfOpts cfOptions

// configPath 是 -config 指定的 JSON 配置文件路径
var configPath string

// fileConfig 是 -config 指向的 JSON 配置文件结构，所有字段都可选。
// 字段一律用指针：只有指针才能区分「没写」与「显式写了零值」，而 flag 优先级判断正依赖这个区分。
type fileConfig struct {
	URL             *string  `json:"url"`
	Referer         *string  `json:"referer"`
	HttpingURL      *string  `json:"httping_url"`
	Routines        *int     `json:"routines"`
	PingTimes       *int     `json:"ping_times"`
	TestCount       *int     `json:"test_count"`
	DownloadTime    *int     `json:"download_time"`
	TCPPort         *int     `json:"tcp_port"`
	Httping         *bool    `json:"httping"`
	HttpingCode     *int     `json:"httping_code"`
	CFColo          *string  `json:"cfcolo"`
	MaxDelay        *int     `json:"max_delay"`
	MinDelay        *int     `json:"min_delay"`
	MaxLossRate     *float64 `json:"max_loss_rate"`
	MinSpeed        *float64 `json:"min_speed"`
	PrintNum        *int     `json:"print_num"`
	IPFile          *string  `json:"ip_file"`
	IPText          *string  `json:"ip_text"`
	Output          *string  `json:"output"`
	DisableDownload *bool    `json:"disable_download"`
	TestAll         *bool    `json:"test_all"`
	Debug           *bool    `json:"debug"`
	Cloudflare      *struct {
		Zone    *string `json:"zone"`
		Record  *string `json:"record"`
		Token   *string `json:"token"`
		Count   *int    `json:"count"`
		TTL     *int    `json:"ttl"`
		Proxied *bool   `json:"proxied"`
	} `json:"cloudflare"`
}

// fatal 打印带中文原因的 [错误] 并以非 0 退出 —— 静默退出会让人以为程序在正常跑
func fatal(format string, args ...interface{}) {
	utils.Red.Printf("[错误] "+format+"\n", args...)
	os.Exit(1)
}

// applyConfigFile 把 -config 配置文件里的值套到「命令行没有显式给出」的参数上。
// 「有没有显式给出」用 flag.Visit 判定，不能拿值和默认值比较 —— 那会把用户显式写的默认值当成没写。
func applyConfigFile(path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("配置文件读取失败: %v", err)
	}
	var fc fileConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields() // 拼错字段名时直接报错，而不是静默忽略
	if err := decoder.Decode(&fc); err != nil {
		fatal("配置文件解析失败: %v", err)
	}

	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	if !set["url"] && fc.URL != nil {
		task.URL = *fc.URL
	}
	if !set["referer"] && fc.Referer != nil {
		task.Referer = *fc.Referer
	}
	if !set["httping-url"] && fc.HttpingURL != nil {
		task.HttpingURL = *fc.HttpingURL
	}
	if !set["n"] && fc.Routines != nil {
		task.Routines = *fc.Routines
	}
	if !set["t"] && fc.PingTimes != nil {
		task.PingTimes = *fc.PingTimes
	}
	if !set["dn"] && fc.TestCount != nil {
		task.TestCount = *fc.TestCount
	}
	if !set["dt"] && fc.DownloadTime != nil {
		downloadTime = *fc.DownloadTime
	}
	if !set["tp"] && fc.TCPPort != nil {
		task.TCPPort = *fc.TCPPort
	}
	if !set["httping"] && fc.Httping != nil {
		task.Httping = *fc.Httping
	}
	if !set["httping-code"] && fc.HttpingCode != nil {
		task.HttpingStatusCode = *fc.HttpingCode
	}
	if !set["cfcolo"] && fc.CFColo != nil {
		task.HttpingCFColo = *fc.CFColo
	}
	if !set["tl"] && fc.MaxDelay != nil {
		maxDelay = *fc.MaxDelay
	}
	if !set["tll"] && fc.MinDelay != nil {
		minDelay = *fc.MinDelay
	}
	if !set["tlr"] && fc.MaxLossRate != nil {
		maxLossRate = *fc.MaxLossRate
	}
	if !set["sl"] && fc.MinSpeed != nil {
		task.MinSpeed = *fc.MinSpeed
	}
	if !set["p"] && fc.PrintNum != nil {
		utils.PrintNum = *fc.PrintNum
	}
	if !set["f"] && fc.IPFile != nil {
		task.IPFile = *fc.IPFile
	}
	if !set["ip"] && fc.IPText != nil {
		task.IPText = *fc.IPText
	}
	if !set["o"] && fc.Output != nil {
		utils.Output = *fc.Output
	}
	if !set["dd"] && fc.DisableDownload != nil {
		task.Disable = *fc.DisableDownload
	}
	if !set["allip"] && fc.TestAll != nil {
		task.TestAll = *fc.TestAll
	}
	if !set["debug"] && fc.Debug != nil {
		utils.Debug = *fc.Debug
	}

	if fc.Cloudflare != nil {
		if !set["cf-zone"] && fc.Cloudflare.Zone != nil {
			cfOpts.zone = *fc.Cloudflare.Zone
		}
		if !set["cf-record"] && fc.Cloudflare.Record != nil {
			cfOpts.record = *fc.Cloudflare.Record
		}
		if !set["cf-token"] && fc.Cloudflare.Token != nil {
			cfOpts.token = *fc.Cloudflare.Token
		}
		if !set["cf-count"] && fc.Cloudflare.Count != nil {
			cfOpts.count = *fc.Cloudflare.Count
		}
		if !set["cf-ttl"] && fc.Cloudflare.TTL != nil {
			cfOpts.ttl = *fc.Cloudflare.TTL
		}
		if !set["cf-proxied"] && fc.Cloudflare.Proxied != nil {
			cfOpts.proxied = *fc.Cloudflare.Proxied
		}
	}
}

// validateOptions 校验合并后的最终参数，非法时带中文原因非 0 退出
func validateOptions() {
	checkURL := func(name, value string) {
		if value == "" {
			return // 空值有各自的语义（例如 -referer 空 = 不发送、-httping-url 空 = 回退 -url）
		}
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			fatal("%s 必须是 http/https 开头: %s", name, value)
		}
	}
	checkURL("测速地址(-url)", task.URL)
	checkURL("HTTPing 测速地址(-httping-url)", task.HttpingURL)

	if task.Routines < 1 || task.Routines > 1000 {
		fatal("-n 必须在 1..1000 之间，当前: %d", task.Routines)
	}
	if task.PingTimes < 1 {
		fatal("-t 必须 >= 1，当前: %d", task.PingTimes)
	}
	if downloadTime < 1 {
		fatal("-dt 必须 >= 1，当前: %d", downloadTime)
	}
	if task.TCPPort < 1 || task.TCPPort > 65535 {
		fatal("-tp 必须在 1..65535 之间，当前: %d", task.TCPPort)
	}

	// token 允许走环境变量，方便 cron 里不把密钥写进配置文件
	if cfOpts.token == "" {
		cfOpts.token = os.Getenv("CF_API_TOKEN")
	}
	provided := 0
	for _, value := range []string{cfOpts.zone, cfOpts.record, cfOpts.token} {
		if value != "" {
			provided++
		}
	}
	if provided == 0 {
		return // 三者全空 = 不启用 DNS 更新
	}
	if provided < 3 {
		var missing []string
		if cfOpts.zone == "" {
			missing = append(missing, "zone")
		}
		if cfOpts.record == "" {
			missing = append(missing, "record")
		}
		if cfOpts.token == "" {
			missing = append(missing, "token（-cf-token 或环境变量 CF_API_TOKEN）")
		}
		fatal("Cloudflare DNS 更新需要同时提供 zone、record、token，当前缺少: %s", strings.Join(missing, "、"))
	}
	if cfOpts.count < 1 || cfOpts.count > 20 {
		fatal("-cf-count 必须在 1..20 之间，当前: %d", cfOpts.count)
	}
	if cfOpts.ttl < 1 || cfOpts.ttl > 86400 {
		fatal("-cf-ttl 必须在 1..86400 之间（1=自动），当前: %d", cfOpts.ttl)
	}
	// zone 是根域名时才校验记录名归属；zone 写成 Zone ID 时无从校验
	if !isZoneIDLike(cfOpts.zone) {
		if !recordInZone(cfOpts.record, cfOpts.zone) {
			fatal("记录名 %s 不属于 zone %s", cfOpts.record, cfOpts.zone)
		}
	}
}

// cfEnabled 三要素齐备才启用 DNS 更新（不额外引入布尔开关，避免「零值==默认值」的歧义）
func cfEnabled() bool {
	return cfOpts.zone != "" && cfOpts.record != "" && cfOpts.token != ""
}

// recordInZone 记录名要么就是根记录（等于 zone 或写 @），要么以 .zone 结尾
func recordInZone(record, zone string) bool {
	if record == "@" || strings.EqualFold(record, zone) {
		return true
	}
	return strings.HasSuffix(strings.ToLower(record), "."+strings.ToLower(zone))
}

// isZoneIDLike 与 cfdns 的判定保持一致：32 位十六进制视为 Zone ID
func isZoneIDLike(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// maskToken 只保留前 4 位，避免把 API Token 打进日志/终端
func maskToken(token string) string {
	if token == "" {
		return "(未设置)"
	}
	if len(token) <= 4 {
		return "****"
	}
	return token[:4] + "****"
}

// httpingTargetURL 展示 HTTPing 实际会用的地址（空则回退 -url），与 task 包的取值规则一致
func httpingTargetURL() string {
	if task.HttpingURL != "" {
		return task.HttpingURL
	}
	return task.URL + "（回退 -url）"
}

// printEffectiveConfig 打印最终生效的完整配置，便于事后核对「跑的是不是我以为的那套参数」
func printEffectiveConfig() {
	fmt.Println("[配置] 下载测速地址: " + task.URL)
	referer := task.Referer
	if referer == "" {
		referer = "(不发送)"
	}
	fmt.Println("[配置] 下载 Referer: " + referer)
	fmt.Println("[配置] HTTPing 地址: " + httpingTargetURL())
	fmt.Printf("[配置] 延迟线程/次数: %d / %d\n", task.Routines, task.PingTimes)
	fmt.Printf("[配置] 端口: %d\n", task.TCPPort)
	fmt.Printf("[配置] 下载测速: 数量 %d / 时间 %ds / 下限 %.2f MB/s\n", task.TestCount, downloadTime, task.MinSpeed)
	fmt.Printf("[配置] 延迟条件: %d ~ %d ms, 丢包上限 %.2f\n", minDelay, maxDelay, maxLossRate)
	fmt.Printf("[配置] 显示数量/输出文件: %d / %s\n", utils.PrintNum, utils.Output)
	if cfEnabled() {
		fmt.Printf("[配置] Cloudflare DNS: 启用 (zone=%s, record=%s, token=%s, count=%d, ttl=%d, proxied=%v)\n",
			cfOpts.zone, cfOpts.record, maskToken(cfOpts.token), cfOpts.count, cfOpts.ttl, cfOpts.proxied)
	} else {
		fmt.Println("[配置] Cloudflare DNS: 未启用")
	}
}

// updateDNS 把测速结果里最快的若干个 IP 同步到 Cloudflare DNS。
// 候选给到 count 的两倍，让 cfdns 内部按 IP 家族各自取前 count 条。
func updateDNS(speedData utils.DownloadSpeedSet) {
	if !cfEnabled() {
		return
	}
	limit := cfOpts.count * 2
	if limit > len(speedData) {
		limit = len(speedData)
	}
	ips := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		ips = append(ips, speedData[i].IP.String())
	}
	if len(ips) == 0 {
		utils.Yellow.Println("[警告] Cloudflare DNS 更新跳过：没有可用 IP。")
		return
	}
	result, err := cfdns.Update(cfdns.Config{
		Zone:    cfOpts.zone,
		Record:  cfOpts.record,
		Token:   cfOpts.token,
		TTL:     cfOpts.ttl,
		Proxied: cfOpts.proxied,
		Count:   cfOpts.count,
	}, ips)
	if err != nil {
		utils.Red.Printf("[错误] Cloudflare DNS 更新失败: %v\n", err)
		os.Exit(1) // 非 0 退出，便于 cron 告警
	}
	if result.Skipped {
		utils.Cyan.Println("[信息] Cloudflare DNS 已是最新，未做改动。")
		return
	}
	utils.Cyan.Printf("[信息] Cloudflare DNS 更新完成：创建 %d 条，更新 %d 条，删除 %d 条。\n",
		result.Created, result.Updated, result.Deleted)
}
