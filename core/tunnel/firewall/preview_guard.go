package firewall

import (
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"syscall"
	"unsafe"
)

// This namespace is exclusive to Pinus Preview. No production or third-party
// provider is enumerated for deletion. All replacements use one transaction.
var previewGuardProvider = windows.GUID{Data1: 0x358f11b2, Data2: 0x2c6e, Data3: 0x4dad, Data4: [8]byte{0x98, 0x63, 0xd4, 0xd9, 0x2e, 0x59, 0xca, 0x40}}
var previewGuardSublayer = windows.GUID{Data1: 0x9b3a2aa9, Data2: 0x058b, Data3: 0x46bd, Data4: [8]byte{0x88, 0x03, 0x0b, 0x92, 0x64, 0x0b, 0x75, 0x36}}
var guardDLL = windows.NewLazySystemDLL("fwpuclnt.dll")
var guardEnumCreate = guardDLL.NewProc("FwpmFilterCreateEnumHandle0")
var guardEnum = guardDLL.NewProc("FwpmFilterEnum0")
var guardEnumDestroy = guardDLL.NewProc("FwpmFilterDestroyEnumHandle0")
var guardDelete = guardDLL.NewProc("FwpmFilterDeleteByKey0")

func guardCall(proc *windows.LazyProc, args ...uintptr) error {
	result, _, _ := proc.Call(args...)
	if result != 0 {
		return syscall.Errno(result)
	}
	return nil
}
func guardOpen() (uintptr, error) {
	session := wtFwpmSession0{txnWaitTimeoutInMSec: 5000}
	var handle uintptr
	err := fwpmEngineOpen0(nil, cRPC_C_AUTHN_WINNT, nil, &session, unsafe.Pointer(&handle))
	return handle, err
}
func guardOwned(filter *wtFwpmFilter0) bool {
	return filter != nil && filter.providerKey != nil && *filter.providerKey == previewGuardProvider && filter.subLayerKey == previewGuardSublayer
}
func clearGuardFilters(session uintptr) error {
	var enumerator uintptr
	if err := guardCall(guardEnumCreate, session, 0, uintptr(unsafe.Pointer(&enumerator))); err != nil {
		return err
	}
	defer guardCall(guardEnumDestroy, session, enumerator)
	var keys []windows.GUID
	for {
		var entries **wtFwpmFilter0
		var count uint32
		if err := guardCall(guardEnum, session, enumerator, 128, uintptr(unsafe.Pointer(&entries)), uintptr(unsafe.Pointer(&count))); err != nil {
			return err
		}
		if count > 128 || (count > 0 && entries == nil) {
			if entries != nil {
				fwpmFreeMemory0(unsafe.Pointer(&entries))
			}
			return errors.New("invalid WFP enumeration")
		}
		for _, filter := range unsafe.Slice(entries, int(count)) {
			if guardOwned(filter) {
				keys = append(keys, filter.filterKey)
			}
		}
		if entries != nil {
			fwpmFreeMemory0(unsafe.Pointer(&entries))
		}
		if count < 128 {
			break
		}
	}
	for i := range keys {
		if err := guardCall(guardDelete, session, uintptr(unsafe.Pointer(&keys[i]))); err != nil {
			return err
		}
	}
	runtime.KeepAlive(keys)
	return nil
}

func guardFilterAdd(session uintptr, filter *wtFwpmFilter0, sd uintptr, id *uint64) error {
	if guardOwned(filter) {
		// Interface LUIDs may be reassigned across boots. Never persist a TUN
		// permit: the persistent block survives; the manager recreates the permit.
		persistent := true
		for _, condition := range unsafe.Slice(filter.filterCondition, int(filter.numFilterConditions)) {
			if condition.fieldKey == cFWPM_CONDITION_IP_LOCAL_INTERFACE {
				persistent = false
			}
		}
		if persistent {
			filter.flags |= cFWPM_FILTER_FLAG_PERSISTENT
		}
	}
	return fwpmFilterAdd0(session, filter, sd, id)
}

// SetPreviewGuard blocks physical traffic except the privileged engine,
// manager, loopback and link maintenance. Engine direct exceptions work while
// the engine is alive. The persistent block survives a manager crash/BFE restart;
// it is not a boot-time filter and makes no pre-BFE protection claim.
func SetPreviewGuard(enginePath string, luid uint64) error {
	session, err := guardOpen()
	if err != nil {
		return err
	}
	defer fwpmEngineClose0(session)
	return runTransaction(session, func(session uintptr) error {
		display, err := createWtFwpmDisplayData0("Pinus Preview protection", "Explicit persistent connection protection")
		if err != nil {
			return err
		}
		provider := wtFwpmProvider0{providerKey: previewGuardProvider, displayData: *display, flags: 1}
		if err = fwpmProviderAdd0(session, &provider, 0); err != nil && !errors.Is(err, syscall.Errno(0x80320009)) {
			return err
		}
		sublayer := wtFwpmSublayer0{subLayerKey: previewGuardSublayer, displayData: *display, providerKey: &previewGuardProvider, flags: cFWPM_SUBLAYER_FLAG_PERSISTENT, weight: ^uint16(0)}
		if err = fwpmSubLayerAdd0(session, &sublayer, 0); err != nil && !errors.Is(err, syscall.Errno(0x80320009)) {
			return err
		}
		if err = clearGuardFilters(session); err != nil {
			return err
		}
		bo := &baseObjects{provider: previewGuardProvider, filters: previewGuardSublayer}
		if err = permitExecutable(session, bo, 15, enginePath); err != nil {
			return err
		}
		if err = permitWireGuardService(session, bo, 15); err != nil {
			return err
		}
		if err = permitLoopback(session, bo, 13); err != nil {
			return err
		}
		if luid != 0 {
			if err = permitTunInterface(session, bo, 12, luid); err != nil {
				return err
			}
		}
		if err = permitDHCPIPv4(session, bo, 12); err != nil {
			return err
		}
		if err = permitDHCPIPv6(session, bo, 12); err != nil {
			return err
		}
		if err = permitNdp(session, bo, 12); err != nil {
			return err
		}
		return blockAll(session, bo, 0)
	})
}
func ClearPreviewGuard() error {
	session, err := guardOpen()
	if err != nil {
		return err
	}
	defer fwpmEngineClose0(session)
	return runTransaction(session, clearGuardFilters)
}
