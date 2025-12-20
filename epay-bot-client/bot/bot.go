package bot

import (
	"epay-bot-go/db"
	"epay-bot-go/model"
	"epay-bot-go/service"
	"fmt"
	"log"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

type State int

const (
	StateIdle State = iota
	StateWaitingForDomain
	StateWaitingForPid
	StateWaitingForKey
	StateWaitingForDomainChange
	StateWaitingForPidChange
	StateWaitingForKeyChange
)

type Bot struct {
	b             *tele.Bot
	db            *db.DB
	epay          *service.EpayService
	poller        *service.PollerManager
	userStates    map[int64]State
	tempData      map[int64]map[string]string
	mu            sync.RWMutex
}

func NewBot(token string, database *db.DB, epay *service.EpayService) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		b:          b,
		db:         database,
		epay:       epay,
		userStates: make(map[int64]State),
		tempData:   make(map[int64]map[string]string),
	}

	bot.poller = service.NewPollerManager(database, epay, bot)
	bot.setupHandlers()

	return bot, nil
}

func (bot *Bot) Start() {
	go bot.poller.Start()
	log.Println("Bot started")
	bot.b.Start()
}

func (bot *Bot) Stop() {
	bot.poller.Stop()
	bot.b.Stop()
}

// Implement Notifier interface
func (bot *Bot) NotifyOrder(chatID int64, order model.Order) {
	money := order.Money
	timeStr := order.Endtime
	if timeStr == "" {
		timeStr = order.Addtime
	}
	if timeStr == "" {
		timeStr = "未知时间"
	}

	msg := fmt.Sprintf("🔔 *新订单支付成功通知*\n\n"+
		"🔢 订单号: `%s`\n"+
		"💰 金额: ¥%s\n"+
		"⏱️ 支付时间: %s\n",
		order.TradeNo, money, timeStr)

	_, err := bot.b.Send(&tele.User{ID: chatID}, msg, tele.ModeMarkdown)
	if err != nil {
		log.Printf("Failed to send order notification to %d: %v", chatID, err)
	}
}

func (bot *Bot) NotifySettlement(chatID int64, settlement model.Settlement) {
	money := settlement.Money
	realMoney := settlement.Realmoney
	timeStr := settlement.Endtime
	if timeStr == "" {
		timeStr = settlement.Addtime
	}
	if timeStr == "" {
		timeStr = "未知时间"
	}

	msg := fmt.Sprintf("💵 *新结算成功通知*\n\n"+
		"🆔 结算ID: `%s`\n"+
		"💰 结算金额: ¥%s\n"+
		"💸 实际金额: ¥%s\n"+
		"👤 账户: `%s`\n"+
		"⏱️ 结算时间: %s\n",
		settlement.ID, money, realMoney, settlement.Account, timeStr)

	_, err := bot.b.Send(&tele.User{ID: chatID}, msg, tele.ModeMarkdown)
	if err != nil {
		log.Printf("Failed to send settlement notification to %d: %v", chatID, err)
	}
}

func (bot *Bot) setState(chatID int64, state State) {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.userStates[chatID] = state
}

func (bot *Bot) getState(chatID int64) State {
	bot.mu.RLock()
	defer bot.mu.RUnlock()
	return bot.userStates[chatID]
}

func (bot *Bot) setTempData(chatID int64, key, value string) {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	if bot.tempData[chatID] == nil {
		bot.tempData[chatID] = make(map[string]string)
	}
	bot.tempData[chatID][key] = value
}

func (bot *Bot) getTempData(chatID int64, key string) string {
	bot.mu.RLock()
	defer bot.mu.RUnlock()
	if data, ok := bot.tempData[chatID]; ok {
		return data[key]
	}
	return ""
}

func (bot *Bot) clearTempData(chatID int64) {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	delete(bot.tempData, chatID)
}
