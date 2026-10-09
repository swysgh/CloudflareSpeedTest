package cfhosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeHosts 在 t.TempDir() 里造一个 hosts 文件，返回其路径
func writeHosts(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入测试 hosts 失败: %v", err)
	}
	return path
}

// readHosts 读取 hosts 全文，出错直接终止用例
func readHosts(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取测试 hosts 失败: %v", err)
	}
	return string(data)
}

// §2.1 已有 108.162.198.126 cfip → 该行消失，末尾出现正确 IP 的托管块
func TestUpdateReplacesExistingMapping(t *testing.T) {
	path := writeHosts(t, "127.0.0.1 localhost\n108.162.198.126  cfip\n")

	result, err := Update([]string{"1.1.1.1"}, Config{Domain: "cfip", File: path, Count: 1})
	if err != nil {
		t.Fatalf("Update 返回错误: %v", err)
	}
	got := readHosts(t, path)
	if strings.Contains(got, "108.162.198.126") {
		t.Errorf("旧的 cfip 映射应被删除，实际内容:\n%s", got)
	}
	if !strings.Contains(got, "# cfst begin (auto-generated, do not edit)") {
		t.Errorf("应写入托管块起始标记，实际内容:\n%s", got)
	}
	if !strings.Contains(got, "1.1.1.1  cfip") {
		t.Errorf("托管块应写入 1.1.1.1  cfip（两个空格），实际内容:\n%s", got)
	}
	if result.Added != 1 || result.Removed != 1 || result.Skipped {
		t.Errorf("Result 计数不符: %+v", result)
	}
}

// §2.2 同样输入再跑一次 → Skipped=true，内容与 mtime 都不变
func TestUpdateIsIdempotent(t *testing.T) {
	path := writeHosts(t, "127.0.0.1 localhost\n108.162.198.126  cfip\n")

	if _, err := Update([]string{"1.1.1.1"}, Config{Domain: "cfip", File: path, Count: 1}); err != nil {
		t.Fatalf("首次 Update 返回错误: %v", err)
	}
	beforeContent := readHosts(t, path)
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}

	result, err := Update([]string{"1.1.1.1"}, Config{Domain: "cfip", File: path, Count: 1})
	if err != nil {
		t.Fatalf("第二次 Update 返回错误: %v", err)
	}
	if !result.Skipped {
		t.Errorf("内容一致时 Skipped 应为 true，实际: %+v", result)
	}
	if got := readHosts(t, path); got != beforeContent {
		t.Errorf("跳过时文件内容不应变化:\n前: %q\n后: %q", beforeContent, got)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Errorf("跳过时 mtime 不应变化: %v -> %v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}

// §2.3 域名精确匹配：cfip2 / my-cfip 不能被删
func TestUpdateDomainExactMatch(t *testing.T) {
	path := writeHosts(t, "1.1.1.1 cfip2\n1.1.1.1 my-cfip\n")

	if _, err := Update([]string{"2.2.2.2"}, Config{Domain: "cfip", File: path, Count: 1}); err != nil {
		t.Fatalf("Update 返回错误: %v", err)
	}
	got := readHosts(t, path)
	if !strings.Contains(got, "1.1.1.1 cfip2") {
		t.Errorf("cfip2 不应被删除，实际内容:\n%s", got)
	}
	if !strings.Contains(got, "1.1.1.1 my-cfip") {
		t.Errorf("my-cfip 不应被删除，实际内容:\n%s", got)
	}
}

// §2.4 已有旧托管块（IP 不同）→ 旧块被完整清除，只留一个新块
func TestUpdateClearsOldManagedBlock(t *testing.T) {
	path := writeHosts(t, "127.0.0.1 localhost\n# cfst begin (auto-generated, do not edit)\n9.9.9.9  cfip\n# cfst end\n")

	if _, err := Update([]string{"1.1.1.1"}, Config{Domain: "cfip", File: path, Count: 1}); err != nil {
		t.Fatalf("Update 返回错误: %v", err)
	}
	got := readHosts(t, path)
	if n := strings.Count(got, "# cfst begin"); n != 1 {
		t.Errorf("托管块起始标记应只出现 1 次，实际 %d 次，内容:\n%s", n, got)
	}
	if strings.Contains(got, "9.9.9.9") {
		t.Errorf("旧块里的 IP 应被清除，实际内容:\n%s", got)
	}
	if !strings.Contains(got, "1.1.1.1  cfip") {
		t.Errorf("应写入新块，实际内容:\n%s", got)
	}
}

// §2.5 多行命中：两行都映射 cfip → 两行都被删
func TestUpdateRemovesAllMatchingLines(t *testing.T) {
	path := writeHosts(t, "1.1.1.1  cfip\n2.2.2.2  cfip\n")

	result, err := Update([]string{"3.3.3.3"}, Config{Domain: "cfip", File: path, Count: 1})
	if err != nil {
		t.Fatalf("Update 返回错误: %v", err)
	}
	got := readHosts(t, path)
	if strings.Contains(got, "1.1.1.1") || strings.Contains(got, "2.2.2.2") {
		t.Errorf("两行旧映射都应被删除，实际内容:\n%s", got)
	}
	if result.Removed != 2 {
		t.Errorf("Removed 应为 2，实际: %d", result.Removed)
	}
}

// §2.6 IPv4+IPv6 混合、Count=2 → 托管块里 IPv4 两行、IPv6 两行
func TestUpdateMixedFamiliesCountTwo(t *testing.T) {
	path := writeHosts(t, "127.0.0.1 localhost\n")

	ips := []string{"1.1.1.1", "2606:4700::1", "2.2.2.2", "2606:4700::2", "3.3.3.3", "2606:4700::3"}
	result, err := Update(ips, Config{Domain: "cfip", File: path, Count: 2})
	if err != nil {
		t.Fatalf("Update 返回错误: %v", err)
	}
	got := readHosts(t, path)
	for _, want := range []string{"1.1.1.1  cfip", "2.2.2.2  cfip", "2606:4700::1  cfip", "2606:4700::2  cfip"} {
		if !strings.Contains(got, want) {
			t.Errorf("托管块应包含 %q，实际内容:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"3.3.3.3", "2606:4700::3"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("超过 Count=2 的 %q 不应写入，实际内容:\n%s", unwanted, got)
		}
	}
	if result.Added != 4 {
		t.Errorf("Added 应为 4，实际: %d", result.Added)
	}
	// IPv4 必须排在 IPv6 之前
	if strings.Index(got, "1.1.1.1  cfip") > strings.Index(got, "2606:4700::1  cfip") {
		t.Errorf("应先写 IPv4 再写 IPv6，实际内容:\n%s", got)
	}
}

// §2.7 ips 为空 → 返回 error 且文件内容未被改动
func TestUpdateEmptyIPsLeavesFileUntouched(t *testing.T) {
	const original = "127.0.0.1 localhost\n108.162.198.126  cfip\n"
	path := writeHosts(t, original)

	result, err := Update(nil, Config{Domain: "cfip", File: path, Count: 1})
	if err == nil {
		t.Fatal("ips 为空时应返回错误")
	}
	if result != (Result{}) {
		t.Errorf("应返回零值 Result，实际: %+v", result)
	}
	if got := readHosts(t, path); got != original {
		t.Errorf("出错时文件不应被改动:\n前: %q\n后: %q", original, got)
	}
}

// §2.8 被删行上还有其它主机名 → 该名字保留、新 IP 生效、cfip 不再出现在那一行
func TestUpdateKeepsOtherHostnames(t *testing.T) {
	path := writeHosts(t, "1.1.1.1 cfip other.example\n")

	if _, err := Update([]string{"2.2.2.2"}, Config{Domain: "cfip", File: path, Count: 1}); err != nil {
		t.Fatalf("Update 返回错误: %v", err)
	}
	got := readHosts(t, path)
	if !strings.Contains(got, "other.example") {
		t.Errorf("other.example 必须仍在文件里，实际内容:\n%s", got)
	}
	if !strings.Contains(got, "2.2.2.2  other.example") {
		t.Errorf("保留行应改用新 IP 并去掉 cfip，实际内容:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "other.example") && strings.Contains(line, "cfip") {
			t.Errorf("保留行上不应再有 cfip: %q", line)
		}
	}
}

// §2.9 文件末尾无换行 → 追加后不出现粘连
func TestUpdateWithoutTrailingNewline(t *testing.T) {
	path := writeHosts(t, "127.0.0.1 localhost\n108.162.198.126  cfip") // 末尾故意没有换行

	if _, err := Update([]string{"1.1.1.1"}, Config{Domain: "cfip", File: path, Count: 1}); err != nil {
		t.Fatalf("Update 返回错误: %v", err)
	}
	got := readHosts(t, path)
	if strings.Contains(got, "cfip# cfst begin") {
		t.Errorf("追加处出现粘连，实际内容:\n%s", got)
	}
	if !strings.Contains(got, "\n\n# cfst begin (auto-generated, do not edit)\n") {
		t.Errorf("托管块前应补空行，实际内容:\n%q", got)
	}
}

// §2.10 更新后文件 mode 与更新前一致
func TestUpdatePreservesFileMode(t *testing.T) {
	path := writeHosts(t, "108.162.198.126  cfip\n")
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatalf("Chmod 失败: %v", err)
	}

	if _, err := Update([]string{"1.1.1.1"}, Config{Domain: "cfip", File: path, Count: 1}); err != nil {
		t.Fatalf("Update 返回错误: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("更新后权限应保持 0600，实际: %04o", perm)
	}
}

// 额外：只读文件必须报中文错误，且不被改写
func TestUpdateReadOnlyFileFails(t *testing.T) {
	const original = "108.162.198.126  cfip\n"
	path := writeHosts(t, original)
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatalf("Chmod 失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	_, err := Update([]string{"1.1.1.1"}, Config{Domain: "cfip", File: path, Count: 1})
	if err == nil {
		t.Fatal("只读文件应返回错误")
	}
	if !strings.Contains(err.Error(), "无写权限") {
		t.Errorf("错误信息应说明无写权限，实际: %v", err)
	}
	if got := readHosts(t, path); got != original {
		t.Errorf("出错时文件不应被改动:\n前: %q\n后: %q", original, got)
	}
}
