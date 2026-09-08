/* SPDX-License-Identifier: MIT */

package manager

import (
	"log"
	"sync"
)

var powerTransitionMutex sync.Mutex
var powerGeneration uint64

func handleSmartPowerEvent(eventType uint32, runAsync func(func()), pause func() error, resume func()) bool {
	switch eventType {
	case pbtAPMSuspend:
		log.Println("System suspend detected; pausing smart routing while preserving desired state")
		powerTransitionMutex.Lock()
		powerGeneration++
		generation := powerGeneration
		smartSystemSuspended.Store(true)
		powerTransitionMutex.Unlock()
		runAsync(func() {
			powerTransitionMutex.Lock()
			defer powerTransitionMutex.Unlock()
			if generation != powerGeneration {
				return
			}
			if err := pause(); err != nil {
				log.Printf("Unable to pause smart routing for suspend: %v", err)
			}
		})
		return true
	case pbtAPMResumeSuspend, pbtAPMResumeAuto:
		log.Println("System resume detected; scheduling smart route recovery")
		powerTransitionMutex.Lock()
		powerGeneration++
		resume()
		powerTransitionMutex.Unlock()
		return true
	default:
		return false
	}
}
