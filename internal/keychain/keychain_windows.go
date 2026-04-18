//go:build windows

package keychain

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const regRootPath = `Software\VibeknowCLI\keychain`

var safeRegRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeRegistryComponent(s string) string {
	s = strings.ReplaceAll(s, "\\", "_")
	return safeRegRe.ReplaceAllString(s, "_")
}

func registryPathForService(service string) string {
	return regRootPath + `\` + safeRegistryComponent(service)
}

func valueNameForAccount(account string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(account))
}

// dpapiEntropy binds ciphertext to (service, account) to reduce swap/replay
// risks. Empty entropy is legal but we intentionally use deterministic bytes.
func dpapiEntropy(service, account string) *windows.DataBlob {
	data := []byte(service + "\x00" + account)
	if len(data) == 0 {
		return nil
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

func dpapiProtect(pt []byte, entropy *windows.DataBlob) ([]byte, error) {
	var in windows.DataBlob
	if len(pt) > 0 {
		in = windows.DataBlob{Size: uint32(len(pt)), Data: &pt[0]}
	}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer freeDataBlob(&out)
	if out.Data == nil || out.Size == 0 {
		return []byte{}, nil
	}
	buf := unsafe.Slice(out.Data, int(out.Size))
	res := make([]byte, len(buf))
	copy(res, buf)
	return res, nil
}

func dpapiUnprotect(ct []byte, entropy *windows.DataBlob) ([]byte, error) {
	var in windows.DataBlob
	if len(ct) > 0 {
		in = windows.DataBlob{Size: uint32(len(ct)), Data: &ct[0]}
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer freeDataBlob(&out)
	if out.Data == nil || out.Size == 0 {
		return []byte{}, nil
	}
	buf := unsafe.Slice(out.Data, int(out.Size))
	res := make([]byte, len(buf))
	copy(res, buf)
	return res, nil
}

func freeDataBlob(b *windows.DataBlob) {
	if b == nil || b.Data == nil {
		return
	}
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	b.Data = nil
	b.Size = 0
}

func platformGet(service, account string) ([]byte, error) {
	keyPath := registryPathForService(service)
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return nil, ErrNotFound
	}
	defer k.Close()
	b64, _, err := k.GetStringValue(valueNameForAccount(account))
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if b64 == "" {
		return nil, ErrNotFound
	}
	blob, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	return dpapiUnprotect(blob, dpapiEntropy(service, account))
}

func platformSet(service, account string, data []byte) error {
	protected, err := dpapiProtect(data, dpapiEntropy(service, account))
	if err != nil {
		return fmt.Errorf("dpapi protect: %w", err)
	}
	keyPath := registryPathForService(service)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry create/open: %w", err)
	}
	defer k.Close()
	return k.SetStringValue(valueNameForAccount(account), base64.StdEncoding.EncodeToString(protected))
}

func platformRemove(service, account string) error {
	keyPath := registryPathForService(service)
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	_ = k.DeleteValue(valueNameForAccount(account))
	return nil
}
