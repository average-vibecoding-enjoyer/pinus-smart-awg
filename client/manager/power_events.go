/* SPDX-License-Identifier: MIT */

package manager

import "log"

func handleSmartPowerEvent(eventType uint32, runAsync func(func()), pause func() error, resume func()) bool {
	switch eventType {
	case pbtAPMSuspend:
		log.Println("System suspend detected; pausing smart routing while preserving desired state")
		runAsync(func() {
			if err := pause(); err != nil {
				log.Printf("Unable to pause smart routing for suspend: %v", err)
			}
		})
		return true
	case pbtAPMResumeSuspend, pbtAPMResumeAuto:
		log.Println("System resume detected; scheduling smart route recovery")
		resume()
		return true
	default:
		return false
	}
}
