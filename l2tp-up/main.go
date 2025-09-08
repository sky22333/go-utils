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
	TOKEN_QUERY         = 0x0008
	TOKEN_ELEVATION     = 20
	SW_HIDE            = 0
	SW_SHOW            = 5
	ERROR_ELEVATION_REQUIRED = 740
)

type tokenElevation struct {
	TokenIsElevated uint32
}

func main() {
	fmt.Println("Windows L2TP配置工具")
	
	// 检查是否以管理员权限运行
	if !isRunningAsAdmin() {
		fmt.Println("需要管理员权限，正在请求提升权限...")
		requestAdminPrivileges()
		return
	}

	fmt.Println("管理员权限确认，开始配置...")

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

// 改进的管理员权限检测
func isRunningAsAdmin() bool {
	// 尝试执行一个需要管理员权限的操作来测试
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err == nil
}

// 请求管理员权限
func requestAdminPrivileges() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("获取可执行文件路径失败: %v\n", err)
		fmt.Println("按任意键退出...")
		fmt.Scanln()
		return
	}
	
	// 使用 runas 动词启动具有管理员权限的新实例
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
	
	// 检查 ShellExecute 是否成功
	if ret <= 32 {
		fmt.Println("权限提升被取消或失败")
		fmt.Println("按任意键退出...")
		fmt.Scanln()
	}
}

func configureFirewall() bool {
	fmt.Println("步骤1: 配置防火墙...")
	
	// 先删除可能存在的同名规则
	cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=L2TP-Out")
	cmd.Run() // 忽略错误
	
	// 添加新规则
	cmd = exec.Command("netsh", "advfirewall", "firewall", "add", "rule", 
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
	fmt.Println("步骤2: 配置IPsec服务...")
	
	// 设置服务启动类型为自动
	cmd := exec.Command("sc", "config", "PolicyAgent", "start=auto")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("   配置服务失败: %v\n", err)
		return false
	}
	
	// 尝试启动服务
	cmd = exec.Command("sc", "start", "PolicyAgent")
	err = cmd.Run()
	if err != nil {
		// 检查服务是否已经在运行
		cmd = exec.Command("sc", "query", "PolicyAgent")
		if cmd.Run() == nil {
			fmt.Println("   成功")
			return true
		} else {
			fmt.Printf("   启动服务失败: %v\n", err)
			return false
		}
	}
	
	fmt.Println("   成功")
	return true
}

func openRegistryKey() bool {
	fmt.Println("步骤3: 访问注册表...")
	
	// 尝试打开注册表项以验证访问权限
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
		registry.SET_VALUE)
	if err != nil {
		fmt.Printf("   打开注册表失败: %v\n", err)
		return false
	}
	defer key.Close()
	
	err = key.SetDWordValue("AllowL2TPWeakCrypto", 1)
	if err != nil {
		fmt.Printf("   设置值失败: %v\n", err)
		return false
	}
	
	fmt.Println("   成功")
	return true
}

func createProhibitIPSec() bool {
	fmt.Println("步骤5: 创建ProhibitIpSec...")
	
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, 
		`System\CurrentControlSet\Services\Rasman\Parameters`, 
		registry.SET_VALUE)
	if err != nil {
		fmt.Printf("   打开注册表失败: %v\n", err)
		return false
	}
	defer key.Close()
	
	err = key.SetDWordValue("ProhibitIpSec", 1)
	if err != nil {
		fmt.Printf("   创建值失败: %v\n", err)
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