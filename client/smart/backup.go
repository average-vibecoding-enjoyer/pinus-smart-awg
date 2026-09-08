package smart

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows/conf"
	"golang.org/x/crypto/scrypt"
)

const backupLimit = 16 * 1024 * 1024

var backupMagic = []byte("PINUS-BACKUP\x01")

type BackupProfile struct {
	Name     string
	Config   string
	Settings RoutingSettings
}
type Backup struct {
	Version   int
	CreatedAt time.Time
	Profiles  []BackupProfile
}

func validateBackup(backup Backup) error {
	if backup.Version != 1 || len(backup.Profiles) == 0 || len(backup.Profiles) > 100 {
		return errors.New("некорректная версия или количество профилей")
	}
	seen := map[string]bool{}
	for _, profile := range backup.Profiles {
		if !conf.TunnelNameIsValid(profile.Name) || seen[profile.Name] {
			return errors.New("некорректное или повторяющееся имя профиля")
		}
		seen[profile.Name] = true
		if len(profile.Config) > 1024*1024 {
			return errors.New("профиль превышает 1 МБ")
		}
		if _, err := conf.FromWgQuickWithUnknownEncoding(profile.Config, profile.Name); err != nil {
			return fmt.Errorf("%s: %w", profile.Name, err)
		}
		if _, err := profile.Settings.Normalized(); err != nil {
			return fmt.Errorf("%s: %w", profile.Name, err)
		}
	}
	return nil
}

func EncryptBackup(backup Backup, password string) ([]byte, error) {
	if len([]rune(password)) < 12 {
		return nil, errors.New("для резервной копии нужен пароль не короче 12 символов")
	}
	if err := validateBackup(backup); err != nil {
		return nil, err
	}
	plain, err := json.Marshal(backup)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	if len(plain) > backupLimit {
		return nil, errors.New("резервная копия превышает 16 МБ")
	}
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(password), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	header := append(append(append([]byte(nil), backupMagic...), salt...), nonce...)
	return gcm.Seal(header, nonce, plain, header), nil
}

func DecryptBackup(data []byte, password string) (Backup, error) {
	var backup Backup
	headerLen := len(backupMagic) + 16 + 12
	if len(data) < headerLen+16 || len(data) > backupLimit+headerLen+16 || !bytes.HasPrefix(data, backupMagic) {
		return backup, errors.New("неподдерживаемый или повреждённый архив Pinus")
	}
	salt := data[len(backupMagic) : len(backupMagic)+16]
	nonce := data[len(backupMagic)+16 : headerLen]
	key, err := scrypt.Key([]byte(password), salt, 32768, 8, 1, 32)
	if err != nil {
		return backup, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return backup, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return backup, err
	}
	plain, err := gcm.Open(nil, nonce, data[headerLen:], data[:headerLen])
	if err != nil {
		return backup, errors.New("неверный пароль или повреждённый архив")
	}
	defer clear(plain)
	if err = decodeStrictJSON(plain, &backup); err != nil {
		return Backup{}, errors.New("некорректное содержимое резервной копии")
	}
	if err = validateBackup(backup); err != nil {
		return Backup{}, err
	}
	return backup, nil
}

// WriteBackupFile commits only a complete encrypted backup.
func WriteBackupFile(path string, encrypted []byte) error { return writeAtomic(path, encrypted) }
