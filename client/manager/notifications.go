package manager

import "time"

func queueLatestEvent(queue chan []byte, event []byte) {
	if queue == nil {
		return
	}
	select {
	case queue <- event:
		return
	default:
	}
	select {
	case <-queue:
	default:
	}
	select {
	case queue <- event:
	default:
	}
}
func (s *ManagerService) pumpEvents() {
	for {
		select {
		case <-s.eventDone:
			return
		case event := <-s.eventQueue:
			s.eventLock.Lock()
			file := s.events
			s.eventLock.Unlock()
			if file == nil {
				return
			}
			file.SetWriteDeadline(time.Now().Add(time.Second))
			// An incomplete gob frame cannot be repaired by sending another frame.
			// Close the stream so the UI recognizes loss of synchronization.
			if n, err := file.Write(event); err != nil || n != len(event) {
				file.Close()
				return
			}
		}
	}
}
