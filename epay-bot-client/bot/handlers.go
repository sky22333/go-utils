package bot

import (
	"epay-bot-go/model"
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func (bot *Bot) setupHandlers() {
	// Commands
	bot.b.Handle("/start", bot.handleStart)
	bot.b.Handle("/menu", bot.handleMenu)
	bot.b.Handle("/help", bot.handleHelp)
	bot.b.Handle("/cancel", bot.handleCancel)

	// Text Input
	bot.b.Handle(tele.OnText, bot.handleText)

	// Callbacks
	bot.b.Handle(&btnSetupMerchant, bot.startMerchantSetup)
	bot.b.Handle(&btnBackToMain, bot.handleBackToMain)
	
	// Since unique IDs are strings, we can handle them by string if we didn't use variable pointers
	// But telebot supports handling by unique string too.
	// Let's use the button variables where possible, or string matching.
	
	bot.b.Handle(&btnModifyInfo, bot.handleModifyInfo)
	bot.b.Handle(&btnModifyDomain, bot.handleModifyDomain)
	bot.b.Handle(&btnModifyPid, bot.handleModifyPid)
	bot.b.Handle(&btnModifyKey, bot.handleModifyKey)
	
	bot.b.Handle(&btnCheckOrders, bot.handleCheckOrders)
	bot.b.Handle(&btnCheckSuccess, bot.handleCheckSuccessOrders)
	bot.b.Handle(&btnCheckSettle, bot.handleCheckSettlements)
	// Toggle polling needs dynamic handling because the button text changes but ID stays same
	bot.b.Handle(&tele.Btn{Unique: "toggle_polling"}, bot.handleTogglePolling)
}

func (bot *Bot) handleStart(c tele.Context) error {
	chatID := c.Chat().ID
	
	merchantInfo := bot.getMerchantInfoText(chatID)
	welcomeText := "👋 欢迎使用易支付订单通知机器人！"
	
	if merchantInfo != "" {
		welcomeText += fmt.Sprintf("\n\n%s", merchantInfo)
	}

	return c.Send(welcomeText, tele.ModeMarkdown, bot.getMainMenuKeyboard(chatID))
}

func (bot *Bot) handleMenu(c tele.Context) error {
	return bot.handleStart(c) // Same logic
}

func (bot *Bot) handleHelp(c tele.Context) error {
	helpText := "📌 *支付查询机器人使用帮助*\n\n" +
		"基本命令：\n" +
		"/start - 启动机器人并显示主菜单\n" +
		"/menu - 显示主菜单\n" +
		"/help - 显示此帮助信息\n" +
		"/cancel - 取消当前操作\n\n" +
		"基本设置：\n" +
		"1. 首先设置商户信息（域名、商户ID和密钥）\n" +
		"2. 设置完成后可以随时修改商户信息\n\n" +
		"功能说明：\n" +
		"- 查询订单：可查看最近30条订单或仅成功订单\n" +
		"- 查询结算：可查看最近结算记录\n" +
		"- 长轮询：开启后自动通知新的成功支付订单和结算记录"
	
	return c.Send(helpText, tele.ModeMarkdown)
}

func (bot *Bot) handleCancel(c tele.Context) error {
	chatID := c.Chat().ID
	bot.setState(chatID, StateIdle)
	bot.clearTempData(chatID)
	
	merchantInfo := bot.getMerchantInfoText(chatID)
	cancelText := "❌ 已取消当前操作。返回主菜单："
	if merchantInfo != "" {
		cancelText = fmt.Sprintf("%s\n\n%s", merchantInfo, cancelText)
	}
	
	return c.Send(cancelText, tele.ModeMarkdown, bot.getMainMenuKeyboard(chatID))
}

func (bot *Bot) handleText(c tele.Context) error {
	chatID := c.Chat().ID
	state := bot.getState(chatID)
	text := strings.TrimSpace(c.Text())

	switch state {
	case StateWaitingForDomain:
		return bot.processDomainInput(c, chatID, text)
	case StateWaitingForPid:
		return bot.processPidInput(c, chatID, text)
	case StateWaitingForKey:
		return bot.processKeyInput(c, chatID, text)
	case StateWaitingForDomainChange:
		return bot.processDomainChange(c, chatID, text)
	case StateWaitingForPidChange:
		return bot.processPidChange(c, chatID, text)
	case StateWaitingForKeyChange:
		return bot.processKeyChange(c, chatID, text)
	}

	return nil
}

// Wizard Steps

func (bot *Bot) startMerchantSetup(c tele.Context) error {
	chatID := c.Chat().ID
	bot.setState(chatID, StateWaitingForDomain)
	bot.clearTempData(chatID)
	
	return c.Edit("🌐 请输入易支付域名\n例如： example.com")
}

func (bot *Bot) processDomainInput(c tele.Context, chatID int64, text string) error {
	if !strings.Contains(text, ".") {
		return c.Send("❌ 域名格式无效！请输入有效的域名。\n例如： example.com")
	}
	
	domain := strings.TrimPrefix(text, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	
	bot.setTempData(chatID, "domain", domain)
	bot.setState(chatID, StateWaitingForPid)
	
	return c.Send("🆔 请输入商户ID\n例如：1000")
}

func (bot *Bot) processPidInput(c tele.Context, chatID int64, text string) error {
	// Simple numeric check could be done here, but let's just accept strings as some might differ
	bot.setTempData(chatID, "pid", text)
	bot.setState(chatID, StateWaitingForKey)
	
	return c.Send("🔑 请输入商户密钥\n例如： da1b2c3d4e5f6g7h8i9j0sddsda")
}

func (bot *Bot) processKeyInput(c tele.Context, chatID int64, text string) error {
	domain := bot.getTempData(chatID, "domain")
	pid := bot.getTempData(chatID, "pid")
	
	if domain == "" || pid == "" {
		bot.setState(chatID, StateIdle)
		return c.Send("❌ 设置过程出错，请重新开始设置商户信息。", bot.getMainMenuKeyboard(chatID))
	}
	
	info := model.MerchantInfo{
		ChatID: chatID,
		Domain: domain,
		Pid:    pid,
		Key:    text,
	}
	
	if err := bot.db.SaveMerchantInfo(info); err != nil {
		return c.Send("❌ 保存失败: " + err.Error())
	}
	
	bot.setState(chatID, StateIdle)
	bot.clearTempData(chatID)
	
	msg := fmt.Sprintf("✅ 商户信息设置成功！\n\n%s", bot.getMerchantInfoText(chatID))
	return c.Send(msg, tele.ModeMarkdown, bot.getMainMenuKeyboard(chatID))
}

// Modification Handlers

func (bot *Bot) handleModifyInfo(c tele.Context) error {
	return c.Edit("请选择要修改的信息：", bot.getModifyMenuKeyboard())
}

func (bot *Bot) handleModifyDomain(c tele.Context) error {
	chatID := c.Chat().ID
	info, _ := bot.db.GetMerchantInfo(chatID)
	current := "未设置"
	if info != nil {
		current = info.Domain
	}
	
	bot.setState(chatID, StateWaitingForDomainChange)
	return c.Edit(fmt.Sprintf("🌐 当前域名: `%s`\n\n请输入新的域名\n例如： example.com", current), tele.ModeMarkdown)
}

func (bot *Bot) processDomainChange(c tele.Context, chatID int64, text string) error {
	if !strings.Contains(text, ".") {
		return c.Send("❌ 域名格式无效！请输入有效的域名。\n例如： example.com")
	}
	domain := strings.TrimPrefix(text, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	
	info, _ := bot.db.GetMerchantInfo(chatID)
	if info == nil {
		return c.Send("❌ 未找到商户信息！请先设置商户信息。", bot.getMainMenuKeyboard(chatID))
	}
	
	info.Domain = domain
	bot.db.SaveMerchantInfo(*info)
	bot.setState(chatID, StateIdle)
	
	return c.Send(fmt.Sprintf("✅ 域名已更新！\n\n%s", bot.getMerchantInfoText(chatID)), tele.ModeMarkdown, bot.getMainMenuKeyboard(chatID))
}

func (bot *Bot) handleModifyPid(c tele.Context) error {
	chatID := c.Chat().ID
	info, _ := bot.db.GetMerchantInfo(chatID)
	current := "未设置"
	if info != nil {
		current = info.Pid
	}
	
	bot.setState(chatID, StateWaitingForPidChange)
	return c.Edit(fmt.Sprintf("🆔 当前商户ID: `%s`\n\n请输入新的商户ID\n例如：1000", current), tele.ModeMarkdown)
}

func (bot *Bot) processPidChange(c tele.Context, chatID int64, text string) error {
	info, _ := bot.db.GetMerchantInfo(chatID)
	if info == nil {
		return c.Send("❌ 未找到商户信息！请先设置商户信息。", bot.getMainMenuKeyboard(chatID))
	}
	
	info.Pid = text
	bot.db.SaveMerchantInfo(*info)
	bot.setState(chatID, StateIdle)
	
	return c.Send(fmt.Sprintf("✅ 商户ID已更新！\n\n%s", bot.getMerchantInfoText(chatID)), tele.ModeMarkdown, bot.getMainMenuKeyboard(chatID))
}

func (bot *Bot) handleModifyKey(c tele.Context) error {
	chatID := c.Chat().ID
	info, _ := bot.db.GetMerchantInfo(chatID)
	current := "未设置"
	if info != nil {
		current = maskKey(info.Key)
	}
	
	bot.setState(chatID, StateWaitingForKeyChange)
	return c.Edit(fmt.Sprintf("🔑 当前密钥: `%s`\n\n请输入新的商户密钥", current), tele.ModeMarkdown)
}

func (bot *Bot) processKeyChange(c tele.Context, chatID int64, text string) error {
	info, _ := bot.db.GetMerchantInfo(chatID)
	if info == nil {
		return c.Send("❌ 未找到商户信息！请先设置商户信息。", bot.getMainMenuKeyboard(chatID))
	}
	
	info.Key = text
	bot.db.SaveMerchantInfo(*info)
	bot.setState(chatID, StateIdle)
	
	return c.Send(fmt.Sprintf("✅ 商户密钥已更新！\n\n%s", bot.getMerchantInfoText(chatID)), tele.ModeMarkdown, bot.getMainMenuKeyboard(chatID))
}

func (bot *Bot) handleBackToMain(c tele.Context) error {
	chatID := c.Chat().ID
	merchantInfo := bot.getMerchantInfoText(chatID)
	menuText := "📋 主菜单 - 请选择一个操作："
	if merchantInfo != "" {
		menuText = fmt.Sprintf("%s\n\n%s", merchantInfo, menuText)
	}
	
	// Use Edit if possible (callback), or Send
	return c.Edit(menuText, tele.ModeMarkdown, bot.getMainMenuKeyboard(chatID))
}

// Logic Handlers

func (bot *Bot) handleTogglePolling(c tele.Context) error {
	chatID := c.Chat().ID
	
	active, _ := bot.db.GetPollingStatus(chatID)
	newStatus := !active
	
	if newStatus {
		// Enable
		bot.db.SetPollingStatus(chatID, true)
		bot.poller.StartPolling(chatID)
		return c.Edit("✅ 订单通知已开启！\n\n您将自动收到新的成功支付订单和结算的通知。", bot.getMainMenuKeyboard(chatID))
	} else {
		// Disable
		bot.db.SetPollingStatus(chatID, false)
		bot.poller.StopPolling(chatID)
		return c.Edit("✅ 订单通知已关闭！\n\n您将不再收到新订单和结算的自动通知。", bot.getMainMenuKeyboard(chatID))
	}
}

func (bot *Bot) handleCheckOrders(c tele.Context) error {
	return bot.checkOrdersCommon(c, false)
}

func (bot *Bot) handleCheckSuccessOrders(c tele.Context) error {
	return bot.checkOrdersCommon(c, true)
}

func (bot *Bot) checkOrdersCommon(c tele.Context, successOnly bool) error {
	chatID := c.Chat().ID
	info, _ := bot.db.GetMerchantInfo(chatID)
	if info == nil {
		return c.Send("❌ 请先设置商户信息")
	}
	
	// Show "Loading..."
	c.Send("🔄 正在查询订单...")
	
	orders, err := bot.epay.GetOrders(info.Domain, info.Pid, info.Key)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ 查询失败: %v", err))
	}
	
	if len(orders) == 0 {
		return c.Send("📭 没有找到订单记录")
	}
	
	msg := "📊 *最近订单列表*\n\n"
	count := 0
	for _, order := range orders {
		status := fmt.Sprintf("%v", order.Status)
		isSuccess := status == "1"
		
		if successOnly && !isSuccess {
			continue
		}
		
		statusEmoji := "❌"
		if isSuccess {
			statusEmoji = "✅"
		}
		
		money := order.Money
		timeStr := order.Endtime
		if timeStr == "" {
			timeStr = order.Addtime
		}
		
		msg += fmt.Sprintf("%s `%s` - ¥%s\n📅 %s\n\n", statusEmoji, order.TradeNo, money, timeStr)
		count++
		if count >= 10 { // Limit to 10 for display
			break 
		}
	}
	
	if count == 0 {
		return c.Send("📭 没有找到符合条件的订单记录")
	}
	
	return c.Send(msg, tele.ModeMarkdown)
}

func (bot *Bot) handleCheckSettlements(c tele.Context) error {
	chatID := c.Chat().ID
	info, _ := bot.db.GetMerchantInfo(chatID)
	if info == nil {
		return c.Send("❌ 请先设置商户信息")
	}
	
	c.Send("🔄 正在查询结算记录...")
	
	settlements, err := bot.epay.GetSettlements(info.Domain, info.Pid, info.Key)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ 查询失败: %v", err))
	}
	
	if len(settlements) == 0 {
		return c.Send("📭 没有找到结算记录")
	}
	
	msg := "💵 *最近结算列表*\n\n"
	count := 0
	for _, s := range settlements {
		status := fmt.Sprintf("%v", s.Status)
		isSuccess := status == "1"
		
		statusEmoji := "❌"
		if isSuccess {
			statusEmoji = "✅"
		}
		
		msg += fmt.Sprintf("%s ID:`%s` - ¥%s\n💸 实到: ¥%s\n📅 %s\n\n", statusEmoji, s.ID, s.Money, s.Realmoney, s.Addtime)
		count++
		if count >= 10 {
			break
		}
	}
	
	return c.Send(msg, tele.ModeMarkdown)
}

// Helpers

func (bot *Bot) getMerchantInfoText(chatID int64) string {
	info, _ := bot.db.GetMerchantInfo(chatID)
	if info == nil {
		return ""
	}
	
	maskedKey := maskKey(info.Key)
	
	return fmt.Sprintf("🔐 *当前商户信息*\n"+
		"🌐 域名: `%s`\n"+
		"🆔 商户ID: `%s`\n"+
		"🔑 密钥: `%s`",
		info.Domain, info.Pid, maskedKey)
}

func maskKey(key string) string {
	if len(key) > 8 {
		return key[:len(key)-8] + "********"
	}
	return "********"
}
