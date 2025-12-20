package bot

import (
	tele "gopkg.in/telebot.v3"
)

var (
	// Buttons
	btnSetupMerchant = tele.Btn{Text: "⚙️ 设置商户信息", Unique: "enter_credentials"}
	
	btnCheckOrders   = tele.Btn{Text: "📊 查询最近30条订单", Unique: "check_all_orders"}
	btnCheckSuccess  = tele.Btn{Text: "✅ 查询成功订单", Unique: "check_success_orders"}
	btnCheckSettle   = tele.Btn{Text: "💵 查询结算记录", Unique: "check_settlements"}
	btnTogglePolling = tele.Btn{Text: "🔄 切换订单通知", Unique: "toggle_polling"}
	btnModifyInfo    = tele.Btn{Text: "⚙️ 修改商户信息", Unique: "modify_merchant_info"}
	btnBackToMain    = tele.Btn{Text: "📋 显示主菜单", Unique: "back_to_main"}

	// Modify Submenu Buttons
	btnModifyDomain = tele.Btn{Text: "🌐 修改域名", Unique: "modify_domain"}
	btnModifyPid    = tele.Btn{Text: "🆔 修改商户ID", Unique: "modify_merchant_id"}
	btnModifyKey    = tele.Btn{Text: "🔑 修改密钥", Unique: "modify_merchant_key"}
	btnBackToMain2  = tele.Btn{Text: "↩️ 返回主菜单", Unique: "back_to_main"} // reusing unique ID
)

func (bot *Bot) getMainMenuKeyboard(chatID int64) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}

	info, _ := bot.db.GetMerchantInfo(chatID)
	hasMerchantInfo := info != nil && info.Pid != "" && info.Key != ""

	if !hasMerchantInfo {
		menu.Inline(
			menu.Row(btnSetupMerchant),
		)
		return menu
	}

	pollingActive, _ := bot.db.GetPollingStatus(chatID)
	pollingText := "🔄 开启订单通知"
	if pollingActive {
		pollingText = "🔄 关闭订单通知"
	}
	
	// Create a dynamic button for polling
	btnToggle := tele.Btn{Text: pollingText, Unique: "toggle_polling"}

	menu.Inline(
		menu.Row(btnCheckOrders),
		menu.Row(btnCheckSuccess),
		menu.Row(btnCheckSettle),
		menu.Row(btnToggle),
		menu.Row(btnModifyInfo),
		menu.Row(btnBackToMain),
	)
	return menu
}

func (bot *Bot) getModifyMenuKeyboard() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(btnModifyDomain),
		menu.Row(btnModifyPid),
		menu.Row(btnModifyKey),
		menu.Row(btnBackToMain2),
	)
	return menu
}
