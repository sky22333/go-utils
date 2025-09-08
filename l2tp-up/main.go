package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	shell32               = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW     = shell32.NewProc("ShellExecuteW")
)

const (
	SW_SHOW = 5
)

func main() {
	fmt.Println("Windows L2TP配置工具")
	
	// 检查是否以管理员权限运行
	if !isRunningAsAdmin() {
		fmt.Println("需要管理员权限，正在请求提升权限...")
		requestAdminPrivileges()
		return
	}

	fmt.Println("管理员权限确认，开始配置...")

	step1Success := configureFirewall()
	step2Success := configureIPSecService()
	step3Success := openRegistryKey()
	step4Success := modifyAllowL2TPWeakCrypto()
	step5Success := createProhibitIPSec()

	fmt.Println("\n配置摘要:")
	fmt.Printf("步骤1: %s | 步骤2: %s | 步骤3: %s | 步骤4: %s | 步骤5: %s\n", 
		getStatusText(step1Success), getStatusText(step2Success), getStatusText(step3Success), 
		getStatusText(step4Success), getStatusText(step5Success))
	
	if step1Success && step2Success && step3Success && step4Success && step5Success {
		fmt.Println("✅ 配置完成")
	} else {
		fmt.Println("⚠️ 部分失败")
	}
	
	fmt.Println("\n按任意键关闭...")
	fmt.Scanln()
}

// 管理员权限检测
func isRunningAsAdmin() bool {
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err == nil
}

func requestAdminPrivileges() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("获取可执行文件路径失败: %v\n", err)
		fmt.Println("按任意键退出...")
		fmt.Scanln()
		return
	}
	
	verb := syscall.StringToUTF16Ptr("runas")
	file := syscall.StringToUTF16Ptr(exe)
	
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		0,
		SW_SHOW,
	)
	
	if ret <= 32 {
		fmt.Println("权限提升被取消或失败")
		fmt.Println("按任意键退出...")
		fmt.Scanln()
	}
}

func configureFirewall() bool {
	fmt.Println("步骤1: 配置防火墙...")
	
	cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=L2TP-Out")
	cmd.Run()
	
	cmd = exec.Command("netsh", "advfirewall", "firewall", "add", "rule", 
		"name=L2TP-Out", "dir=out", "action=allow", "protocol=UDP", "localport=1701")
	
	err := cmd.Run()
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
		return false
	}
	
	cmd = exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name=L2TP-Out")
	err = cmd.Run()
	if err != nil {
		fmt.Println("   失败: 规则未成功添加")
		return false
	}
	
	fmt.Println("   成功")
	return true
}

func configureIPSecService() bool {
	fmt.Println("步骤2: 配置IPsec服务...")
	
	cmd := exec.Command("sc", "config", "PolicyAgent", "start=auto")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("   失败: 配置服务失败 %v\n", err)
		return false
	}
	
	cmd = exec.Command("sc", "start", "PolicyAgent")
	cmd.Run() //
	
	cmd = exec.Command("sc", "query", "PolicyAgent")
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("   失败: 无法查询服务状态 %v\n", err)
		return false
	}
	
	if strings.Contains(string(output), "RUNNING") {
		fmt.Println("   成功")
		return true
	} else if strings.Contains(string(output), "STOPPED") {
		fmt.Println("   成功")
		return true
	} else {
		fmt.Println("   失败: 服务状态异常")
		return false
	}
}

func openRegistryKey() bool {
	fmt.Println("步骤3: 访问注册表...")
	
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, 
		`System\CurrentControlSet\Services\Rasman\Parameters`, 
		registry.QUERY_VALUE)
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
		return false
	}
	defer key.Close()
	
	fmt.Println("   成功")
	return true
}

func modifyAllowL2TPWeakCrypto() bool {
	fmt.Println("步骤4: 修改AllowL2TPWeakCrypto...")
	
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, 
		`System\CurrentControlSet\Services\Rasman\Parameters`, 
		registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		fmt.Printf("   失败: 打开注册表失败 %v\n", err)
		return false
	}
	defer key.Close()
	
	currentValue, _, err := key.GetIntegerValue("AllowL2TPWeakCrypto")
	if err == nil && currentValue == 1 {
		fmt.Println("   成功")
		return true
	}
	
	err = key.SetDWordValue("AllowL2TPWeakCrypto", 1)
	if err != nil {
		fmt.Printf("   失败: 设置值失败 %v\n", err)
		return false
	}
	
	newValue, _, err := key.GetIntegerValue("AllowL2TPWeakCrypto")
	if err != nil || newValue != 1 {
		fmt.Println("   失败: 值未正确设置")
		return false
	}
	
	fmt.Println("   成功")
	return true
}

func createProhibitIPSec() bool {
	fmt.Println("步骤5: 创建ProhibitIpSec...")
	
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, 
		`System\CurrentControlSet\Services\Rasman\Parameters`, 
		registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		fmt.Printf("   失败: 打开注册表失败 %v\n", err)
		return false
	}
	defer key.Close()
	
	currentValue, _, err := key.GetIntegerValue("ProhibitIpSec")
	if err == nil && currentValue == 1 {
		fmt.Println("   成功")
		return true
	}
	
	err = key.SetDWordValue("ProhibitIpSec", 1)
	if err != nil {
		fmt.Printf("   失败: 创建值失败 %v\n", err)
		return false
	}
	
	newValue, _, err := key.GetIntegerValue("ProhibitIpSec")
	if err != nil || newValue != 1 {
		fmt.Println("   失败: 值未正确创建")
		return false
	}
	
	fmt.Println("   成功")
	return true
}

func getStatusText(success bool) string {
	if success {
		return "成功"
	}
	return "失败"
}
