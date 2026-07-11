/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2021 WireGuard LLC. All Rights Reserved.
 */

package conf

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestProtectedDataDirectorySecurityDescriptor(t *testing.T) {
	if _, err := windows.SecurityDescriptorFromString(protectedDataDirectorySDDL); err != nil {
		t.Fatalf("invalid data directory security descriptor: %v", err)
	}

	if protectedDataDirectorySecurityInformation&(windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION) != 0 {
		t.Fatal("data directory ACL update must not attempt to assign an owner or group")
	}
	if protectedDataDirectorySecurityInformation&windows.DACL_SECURITY_INFORMATION == 0 {
		t.Fatal("data directory ACL update must include the DACL")
	}
	if protectedDataDirectorySecurityInformation&windows.PROTECTED_DACL_SECURITY_INFORMATION == 0 {
		t.Fatal("data directory ACL update must protect the DACL from inherited permissions")
	}
}
