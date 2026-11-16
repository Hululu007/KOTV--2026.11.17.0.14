package spider

import (
	"errors"
	"testing"

	"github.com/bobo/KOTV/internal/hostclient"
)

// TestCurrentJarPathPointerByUser：仅测「当前根 jar 路径指针」按用户分桶；
// 磁盘包体仍是公共 JarCache（见 TestPackageCaches_PublicNoUID）。
func TestCurrentJarPathPointerByUser(t *testing.T) {
	t.Cleanup(func() {
		jarPathByUser.Delete("")
		jarPathByUser.Delete("user-jar")
	})
	setCurrentJarPath("/local.jar")
	if got := currentJarPath(); got != "/local.jar" {
		t.Fatalf("local path %q", got)
	}
	leave := hostclient.EnterSession("c-jar", "user-jar", true)
	setCurrentJarPath("/user.jar")
	if got := currentJarPath(); got != "/user.jar" {
		leave()
		t.Fatalf("user path %q", got)
	}
	setCurrentJarPath("")
	if got := currentJarPath(); got != "" {
		leave()
		t.Fatalf("cleared user path %q", got)
	}
	leave()
	if got := currentJarPath(); got != "/local.jar" {
		t.Fatalf("local path changed %q", got)
	}
}

func TestJarRecentSkipWhileBridgeAlive(t *testing.T) {
	prev := bridgeAliveForRecent
	bridgeAliveForRecent = func() bool { return true }
	t.Cleanup(func() {
		bridgeAliveForRecent = prev
		forgetJarRecent("")
		forgetJarRecent("user-recent")
	})
	forgetJarRecent("")
	if jarRecentCanSkip("/a.jar") {
		t.Fatal("未记录 recent")
	}
	rememberJarRecent("/a.jar")
	if !jarRecentCanSkip("/a.jar") {
		t.Fatal("同一路径应跳过")
	}
	if jarRecentCanSkip("/b.jar") {
		t.Fatal("换路径不应跳过")
	}
	bridgeAliveForRecent = func() bool { return false }
	if jarRecentCanSkip("/a.jar") {
		t.Fatal("JVM 不在时不应跳过")
	}
	bridgeAliveForRecent = func() bool { return true }
	leave := hostclient.EnterSession("c-recent", "user-recent", true)
	defer leave()
	if jarRecentCanSkip("/a.jar") {
		t.Fatal("其他用户不应沿用本机 recent")
	}
	rememberJarRecent("/u.jar")
	if !jarRecentCanSkip("/u.jar") {
		t.Fatal("该用户自己的 recent 应跳过")
	}
}

func TestParseJarFailureRetriesNextTime(t *testing.T) {
	const path = "/retry.jar"
	calls := 0
	prev := callParseJar
	callParseJar = func([]byte) error {
		calls++
		if calls == 1 {
			return errors.New("down")
		}
		return nil
	}
	t.Cleanup(func() {
		callParseJar = prev
		parseJarNotified.Delete(jarRunKey(path))
		parseJarFlights.Delete(jarRunKey(path))
	})
	if err := notifyParseJar(path, false); err == nil {
		t.Fatal("第一次应失败")
	}
	if _, ok := parseJarNotified.Load(jarRunKey(path)); ok {
		t.Fatal("失败不应记成已加载")
	}
	if err := notifyParseJar(path, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := parseJarNotified.Load(jarRunKey(path)); !ok {
		t.Fatal("成功后应记上")
	}
	if err := notifyParseJar(path, false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("成功后不应再请求，calls=%d", calls)
	}
}

func TestParseJarNotifiedSeparateByUser(t *testing.T) {
	const path = "/shared.jar"
	calls := 0
	prev := callParseJar
	callParseJar = func([]byte) error {
		calls++
		return nil
	}
	t.Cleanup(func() {
		callParseJar = prev
		parseJarNotified.Delete("\x01" + path)
	})
	if err := notifyParseJar(path, false); err != nil {
		t.Fatal(err)
	}
	leave := hostclient.EnterSession("c-parse", "other-parse", true)
	defer leave()
	other := jarRunKey(path)
	t.Cleanup(func() { parseJarNotified.Delete(other) })
	if err := notifyParseJar(path, false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("其它租户不应跳过 parse，calls=%d", calls)
	}
}

func TestClearJarGoStateForRuntime_PurgesParseNotified(t *testing.T) {
	const path = "/kill-user.jar"
	leave := hostclient.EnterSession("c-ku", "kill-u1", true)
	key := jarRunKey(path)
	parseJarNotified.Store(key, struct{}{})
	jarPathByUser.Store("kill-u1", path)
	rememberJarRecent(path)
	configBaseByClient.Store("u:kill-u1", "http://example.com/")
	leave()

	// 共享侧残留不得被用户清理误删。
	sharedKey := "\x01" + path
	parseJarNotified.Store(sharedKey, struct{}{})
	t.Cleanup(func() { parseJarNotified.Delete(sharedKey) })

	clearJarGoStateForRuntime("kill-u1")
	if _, ok := parseJarNotified.Load(key); ok {
		t.Fatal("parseJarNotified should be cleared for killed user")
	}
	if _, ok := parseJarNotified.Load(sharedKey); !ok {
		t.Fatal("shared parseJarNotified must survive user clear")
	}
	if _, ok := jarPathByUser.Load("kill-u1"); ok {
		t.Fatal("jarPathByUser should be cleared")
	}
	if _, ok := jarRecentByUser.Load("kill-u1"); ok {
		t.Fatal("jarRecent should be cleared")
	}
	if _, ok := configBaseByClient.Load("u:kill-u1"); ok {
		t.Fatal("configBase should be cleared")
	}
}

func TestBumpJarRefreshForCaller_LeavesOtherTenant(t *testing.T) {
	const path = "/bump-caller.jar"
	leaveA := hostclient.EnterSession("c-a", "bump-a", true)
	keyA := jarRunKey(path)
	parseJarNotified.Store(keyA, struct{}{})
	leaveA()
	leaveB := hostclient.EnterSession("c-b", "bump-b", true)
	keyB := jarRunKey(path)
	parseJarNotified.Store(keyB, struct{}{})
	t.Cleanup(func() {
		parseJarNotified.Delete(keyA)
		parseJarNotified.Delete(keyB)
		leaveB()
	})
	url := "https://example.com/bump-only.jar"
	jarNoMD5Refreshed.Store(url, jarRefreshGen.Load())
	otherURL := "https://example.com/other.jar"
	jarNoMD5Refreshed.Store(otherURL, jarRefreshGen.Load())
	t.Cleanup(func() {
		jarNoMD5Refreshed.Delete(url)
		jarNoMD5Refreshed.Delete(otherURL)
	})

	bumpJarRefreshForCaller()
	if _, ok := parseJarNotified.Load(keyB); ok {
		t.Fatal("caller tenant parseJarNotified should clear")
	}
	if _, ok := parseJarNotified.Load(keyA); !ok {
		t.Fatal("other tenant parseJarNotified must survive LoadJar bump")
	}
	// 无 md5 全局条目：LoadJar 只删当前 URL，bumpForCaller 本身不动 jarNoMD5Refreshed。
	if _, ok := jarNoMD5Refreshed.Load(otherURL); !ok {
		t.Fatal("unrelated no-md5 URL must survive caller bump")
	}
}

func TestClearJarGoStateForRuntime_SharedOnly(t *testing.T) {
	const path = "/shared-restart.jar"
	sharedKey := "\x01" + path
	userKey := "keep-u\x01" + path
	parseJarNotified.Store(sharedKey, struct{}{})
	parseJarNotified.Store(userKey, struct{}{})
	jarPathByUser.Store("", path)
	jarPathByUser.Store("keep-u", path)
	t.Cleanup(func() {
		parseJarNotified.Delete(sharedKey)
		parseJarNotified.Delete(userKey)
		jarPathByUser.Delete("")
		jarPathByUser.Delete("keep-u")
	})

	clearJarGoStateForRuntime("")
	if _, ok := parseJarNotified.Load(sharedKey); ok {
		t.Fatal("shared parseJarNotified should be cleared")
	}
	if _, ok := parseJarNotified.Load(userKey); !ok {
		t.Fatal("user parseJarNotified must survive shared clear")
	}
	if _, ok := jarPathByUser.Load(""); ok {
		t.Fatal("shared jar path should be cleared")
	}
	if _, ok := jarPathByUser.Load("keep-u"); !ok {
		t.Fatal("user jar path must survive shared clear")
	}
}
