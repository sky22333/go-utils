package service

import (
	"epay-bot-go/db"
	"epay-bot-go/model"
	"fmt"
	"log"
	"sync"
	"time"
)

type Notifier interface {
	NotifyOrder(chatID int64, order model.Order)
	NotifySettlement(chatID int64, settlement model.Settlement)
}

type PollerManager struct {
	db          *db.DB
	epay        *EpayService
	notifier    Notifier
	jobs        map[int64]*pollJob
	mu          sync.RWMutex
	stopCh      chan struct{}
}

type pollJob struct {
	chatID    int64
	interval  time.Duration
	stop      chan struct{}
	lastOrder time.Time
	lastSettle time.Time
}

func NewPollerManager(database *db.DB, epay *EpayService, notifier Notifier) *PollerManager {
	return &PollerManager{
		db:       database,
		epay:     epay,
		notifier: notifier,
		jobs:     make(map[int64]*pollJob),
		stopCh:   make(chan struct{}),
	}
}

func (pm *PollerManager) Start() {
	// Load all active polling users from DB
	activeChats, err := pm.db.GetActivePollingChats()
	if err != nil {
		log.Printf("Failed to load active polling chats: %v", err)
		return
	}

	for _, chatID := range activeChats {
		pm.StartPolling(chatID)
	}
}

func (pm *PollerManager) Stop() {
	close(pm.stopCh)
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, job := range pm.jobs {
		close(job.stop)
	}
	pm.jobs = make(map[int64]*pollJob)
}

func (pm *PollerManager) StartPolling(chatID int64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.jobs[chatID]; exists {
		return
	}

	job := &pollJob{
		chatID:    chatID,
		interval:  10 * time.Second,
		stop:      make(chan struct{}),
		lastOrder: time.Now(),
		lastSettle: time.Now(),
	}
	pm.jobs[chatID] = job

	go pm.runJob(job)
}

func (pm *PollerManager) StopPolling(chatID int64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if job, exists := pm.jobs[chatID]; exists {
		close(job.stop)
		delete(pm.jobs, chatID)
	}
}

func (pm *PollerManager) runJob(job *pollJob) {
	log.Printf("Polling started for chat %d", job.chatID)
	defer log.Printf("Polling stopped for chat %d", job.chatID)

	ticker := time.NewTicker(job.interval)
	defer ticker.Stop()

	consecutiveErrors := 0
	maxErrors := 5

	for {
		select {
		case <-job.stop:
			return
		case <-pm.stopCh:
			return
		case <-ticker.C:
			// Adjust ticker if interval changed
			// Note: time.Ticker doesn't support dynamic update easily in loop, 
			// usually we just reset it or use time.After/Sleep. 
			// For simplicity and adaptiveness, let's use time.Sleep loop instead of ticker for next iteration
		}

		// Perform check
		hasNew := false
		
		info, err := pm.db.GetMerchantInfo(job.chatID)
		if err != nil || info == nil {
			log.Printf("Merchant info missing for chat %d", job.chatID)
			// Wait a bit
			time.Sleep(60 * time.Second)
			continue
		}

		pm.db.UpdateLastPollTime(job.chatID)

		// Check Orders
		orders, err := pm.epay.GetOrders(info.Domain, info.Pid, info.Key)
		if err != nil {
			log.Printf("Error getting orders for %d: %v", job.chatID, err)
			consecutiveErrors++
		} else {
			for _, order := range orders {
				// Status 1 means success
				status := fmt.Sprintf("%v", order.Status)
				if status == "1" {
					notified, _ := pm.db.IsOrderNotified(order.TradeNo, job.chatID)
					if !notified {
						pm.notifier.NotifyOrder(job.chatID, order)
						pm.db.MarkOrderNotified(order.TradeNo, job.chatID)
						hasNew = true
						job.lastOrder = time.Now()
					}
				}
			}
		}

		// Check Settlements
		settlements, err := pm.epay.GetSettlements(info.Domain, info.Pid, info.Key)
		if err != nil {
			log.Printf("Error getting settlements for %d: %v", job.chatID, err)
			consecutiveErrors++
		} else {
			for _, settle := range settlements {
				status := fmt.Sprintf("%v", settle.Status)
				if status == "1" {
					notified, _ := pm.db.IsSettlementNotified(settle.ID, job.chatID)
					if !notified {
						pm.notifier.NotifySettlement(job.chatID, settle)
						pm.db.MarkSettlementNotified(settle.ID, job.chatID)
						hasNew = true
						job.lastSettle = time.Now()
					}
				}
			}
		}

		// Adaptive interval logic
		if hasNew {
			job.interval = maxDuration(2*time.Second, job.interval/2)
			consecutiveErrors = 0
		} else {
			if time.Since(job.lastOrder) > 5*time.Minute && time.Since(job.lastSettle) > 5*time.Minute {
				job.interval = minDuration(10*time.Second, job.interval+2*time.Second)
			}
		}

		if consecutiveErrors >= maxErrors {
			job.interval = minDuration(60*time.Second, job.interval*2)
			consecutiveErrors = 0 // Reset to avoid infinite backoff? Or keep? Python code resets.
		}

		// Wait for next tick
		ticker.Reset(job.interval)
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
