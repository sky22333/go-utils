package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	shell32               = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW     = shell32.NewProc("ShellExecuteW")
	advapi32              = syscall.NewLazyDLL("advapi32.dll")
	procGetTokenInfo      = advapi32.NewProc("GetTokenInformation")
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentProcess = kernel32.NewProc("GetCurrentProcess")
	procOpenProcessToken  = advapi32.NewProc("OpenProcessToken")
)

const (
	TOKEN_QUERY    = 0x0008
	TokenElevation = 20
	SW_HIDE        = 0
	SW_SHOW        = 5
)

type tokenElevation struct {
	TokenIsElevated uint32
}

func main() {
	fmt.Println("Windows L2TP配置工具")
	
	// 检查是否以管理员权限运行
	if !isAdmin() {
		fmt.Println("正在请求管理员权限...")
		elevatePrivileges()
		return
	}

	fmt.Println("开始配置...")

	// 执行配置步骤
	step1Success := configureFirewall()
	step2Success := configureIPSecService()
	step3Success := openRegistryKey()
	step4Success := modifyAllowL2TPWeakCrypto()
	step5Success := createProhibitIPSec()

	// 显示结果摘要
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

func isAdmin() bool {
	process := procGetCurrentProcess.Addr()
	var token syscall.Handle
	
	ret, _, _ := procOpenProcessToken.Call(
		uintptr(process),
		TOKEN_QUERY,
		uintptr(unsafe.Pointer(&token)),
	)
	
	if ret == 0 {
		return false
	}
	defer syscall.CloseHandle(token)
	
	var elevation tokenElevation
	var returnedLen uint32
	
	ret, _, _ = procGetTokenInfo.Call(
		uintptr(token),
		TokenElevation,
		uintptr(unsafe.Pointer(&elevation)),
		unsafe.Sizeof(elevation),
		uintptr(unsafe.Pointer(&returnedLen)),
	)
	
	if ret == 0 {
		return false
	}
	
	return elevation.TokenIsElevated != 0
}

func elevatePrivileges() {
	exe, _ := os.Executable()
	
	procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("runas"))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(exe))),
		0,
		0,
		SW_SHOW,
	)
}

func configureFirewall() bool {
	fmt.Println("配置防火墙...")
	
	// 使用netsh命令配置防火墙规则
	cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule", 
		"name=L2TP-Out", "dir=out", "action=allow", "protocol=UDP", "localport=1701")
	
	err := cmd.Run()
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
		return false
	}
	
	fmt.Println("   成功")
	return true
}

func configureIPSecService() bool {
	fmt.Println("配置IPsec服务...")
	
	// 设置服务启动类型为自动
	cmd := exec.Command("sc", "config", "PolicyAgent", "start=auto")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
		return false
	}
	
	// 启动服务
	cmd = exec.Command("sc", "start", "PolicyAgent")
	cmd.Run() // 忽略错误，因为服务可能已经在运行
	
	fmt.Println("   成功")
	return true
}

func openRegistryKey() bool {
	fmt.Println("访问注册表...")
	
	// 尝试打开注册表项以验证访问权限
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, 
		`System\CurrentControlSet\Services\Rasman\Parameters`, 
		registry.ALL_ACCESS)
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
		return false
	}
	defer key.Close()
	
	fmt.Println("   成功")
	return true
}

func modifyAllowL2TPWeakCrypto() bool {
	fmt.Println("修改AllowL2TPWeakCrypto...")
	
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, 
		`System\CurrentControlSet\Services\Rasman\Parameters`, 
		registry.SET_VALUE)
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
		return false
	}
	defer key.Close()
	
	err = key.SetDWordValue("AllowL2TPWeakCrypto", 1)
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
		return false
	}
	
	fmt.Println("   成功")
	return true
}

func createProhibitIPSec() bool {
	fmt.Println("创建ProhibitIpSec...")
	
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, 
		`System\CurrentControlSet\Services\Rasman\Parameters`, 
		registry.SET_VALUE)
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
		return false
	}
	defer key.Close()
	
	err = key.SetDWordValue("ProhibitIpSec", 1)
	if err != nil {
		fmt.Printf("   失败: %v\n", err)
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