// Package cfhosts 把测速得到的最优 IP 写入 hosts 文件，让指定域名固定解析到优选 IP。
//
// 与上游的 cfst_hosts.sh 不同：这里按「域名精确匹配」替换，不需要先把 hosts 里的
// 所有旧 CF IP 统一成一个已知 IP，也不会因为按 IP 字符串替换而误伤无关文本。
package cfhosts

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/XIU2/CloudflareSpeedTest/utils"
)

const (
	// defaultFile 是未指定 hosts 文件路径时的默认值
	defaultFile = "/etc/hosts"
	// blockBegin 是托管块起始标记；必须与写入时逐字一致才能被识别并清除
	blockBegin = "# cfst begin (auto-generated, do not edit)"
	// blockEnd 是托管块结束标记
	blockEnd = "# cfst end"
)

// Config 一次 hosts 更新所需的全部参数
type Config struct {
	Domain string // 要写入 hosts 的域名/主机名，如 "cfip"（可以是裸主机名，不一定是 FQDN）
	File   string // hosts 文件路径，如 "/etc/hosts"
	Count  int    // 每个地址族最多写入多少个 IP
}

// Result 一次更新实际发生的动作汇总
type Result struct {
	Added   int  // 本次新增的映射行数
	Removed int  // 本次删除的旧映射行数
	Skipped bool // 内容与目标一致、未改动文件时为 true
}

// Update 把 ips 里每个地址族最快的若干 IP 写入 cfg.Domain 的 hosts 映射。
// ips 必须已按速度从高到低排好序；无法解析的字符串会被忽略。
func Update(ips []string, cfg Config) (Result, error) {
	if cfg.Count <= 0 {
		cfg.Count = 1
	}
	if cfg.File == "" {
		cfg.File = defaultFile
	}

	ipv4, ipv6 := splitByFamily(ips, cfg.Count)
	if len(ipv4) == 0 && len(ipv6) == 0 {
		return Result{}, fmt.Errorf("没有可用的 IP，无法更新 hosts")
	}
	// 保留「误伤行」时用来替换的 IP：优先 IPv4，与托管块的书写顺序一致
	firstIP := ""
	if len(ipv4) > 0 {
		firstIP = ipv4[0]
	} else {
		firstIP = ipv6[0]
	}

	original, err := os.ReadFile(cfg.File)
	if err != nil {
		return Result{}, fmt.Errorf("读取 hosts 文件失败: %w", err)
	}
	info, err := os.Stat(cfg.File)
	if err != nil {
		return Result{}, fmt.Errorf("读取 hosts 文件属性失败: %w", err)
	}

	lines, removed := rewriteLines(string(original), cfg.Domain, firstIP)
	lines = appendManagedBlock(lines, ipv4, ipv6, cfg.Domain)
	newContent := strings.Join(lines, "\n") + "\n"

	// 幂等：内容完全一致时不写文件，保持 mtime 不变
	if newContent == string(original) {
		utils.Cyan.Println("[信息] hosts 已是最新，跳过更新。")
		return Result{Skipped: true}, nil
	}

	if err := atomicWrite(cfg.File, []byte(newContent), info); err != nil {
		return Result{}, err
	}
	result := Result{Added: len(ipv4) + len(ipv6), Removed: removed}
	utils.Green.Printf("[信息] hosts 更新完成：新增 %d 行，删除 %d 行。\n", result.Added, result.Removed)
	return result, nil
}

// rewriteLines 删除所有映射了 domain 的行与旧的托管块，返回处理后的行与删除的行数。
// 若某行除了 domain 还映射了其它主机名，则保留该行、改用 firstIP 并去掉 domain。
func rewriteLines(content, domain, firstIP string) ([]string, int) {
	lines := strings.Split(content, "\n")
	// 去掉末尾换行符产生的空元素，统一在拼接时补回，保证反复运行结果一致
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	out := make([]string, 0, len(lines))
	removed := 0
	inBlock := false
	for _, line := range lines {
		if inBlock {
			if isBlockEnd(line) {
				inBlock = false
			} else if isMappingLine(line) {
				removed++
			}
			continue
		}
		if isBlockBegin(line) {
			inBlock = true
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 { // 非映射行（空行、注释等）原样保留
			out = append(out, line)
			continue
		}
		matched := false
		others := make([]string, 0, len(fields)-1)
		for _, name := range fields[1:] {
			if name == domain { // 精确匹配整个主机名，cfip 不会命中 cfip2 / my-cfip
				matched = true
			} else {
				others = append(others, name)
			}
		}
		if !matched {
			out = append(out, line)
			continue
		}
		if len(others) == 0 {
			removed++
			continue
		}
		// 该行还映射了别的名字，删掉会误伤，保留并改用新 IP
		utils.Yellow.Printf("[警告] hosts 中 %s 所在行还映射了其它主机名，已保留并改用新 IP: %s\n", domain, strings.Join(others, " "))
		out = append(out, firstIP+"  "+strings.Join(others, " "))
	}
	return out, removed
}

// appendManagedBlock 在文件末尾追加托管块：先 IPv4 行再 IPv6 行，块前留一个空行。
func appendManagedBlock(lines []string, ipv4, ipv6 []string, domain string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" { // 归一化块前空行，保证幂等
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, blockBegin)
	for _, ip := range ipv4 {
		lines = append(lines, ip+"  "+domain) // IP 与主机名之间固定两个空格
	}
	for _, ip := range ipv6 {
		lines = append(lines, ip+"  "+domain)
	}
	lines = append(lines, blockEnd)
	return lines
}

// atomicWrite 在同一目录下写临时文件，落盘并同步权限/属主后再原子替换原文件。
// 绝不用 os.WriteFile 直接覆盖 hosts：中途失败会留下半截文件。
func atomicWrite(path string, content []byte, info os.FileInfo) error {
	// 原子替换只要求目录可写，即使文件本身只读（chmod 444）也能成功；
	// 这里显式检查写权限位，保证「只读 hosts」会明确报错而不是被悄悄改写。
	if info.Mode().Perm()&0222 == 0 {
		return fmt.Errorf("hosts 文件无写权限: %s", path)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".cfst-hosts-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 替换成功后该路径已不存在，失败时清理残留

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("同步临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		return fmt.Errorf("设置临时文件权限失败: %w", err)
	}
	// 属主同步只在 root 下做：非 root 通常无权 Chown，直接跳过即可
	if err := chownLike(tmpName, info); err != nil {
		return fmt.Errorf("设置临时文件属主失败: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("替换 hosts 文件失败: %w", err)
	}
	return nil
}

// splitByFamily 按地址族拆分并各取前 count 个，保持传入的速度顺序，无法解析的忽略。
func splitByFamily(ips []string, count int) (ipv4, ipv6 []string) {
	for _, s := range ips {
		ip := net.ParseIP(strings.TrimSpace(s))
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			if len(ipv4) < count {
				ipv4 = append(ipv4, s)
			}
		} else if len(ipv6) < count {
			ipv6 = append(ipv6, s)
		}
		if len(ipv4) >= count && len(ipv6) >= count {
			break
		}
	}
	return
}

func isBlockBegin(line string) bool { return strings.TrimSpace(line) == blockBegin }
func isBlockEnd(line string) bool   { return strings.TrimSpace(line) == blockEnd }

// isMappingLine 判断托管块内的一行是否为「IP + 主机名」映射行（标记行与空行不算）
func isMappingLine(line string) bool {
	fields := strings.Fields(line)
	return len(fields) >= 2 && net.ParseIP(fields[0]) != nil
}
