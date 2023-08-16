package util

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

func CheckFileExists(path string) (bool, error) {
	isAbs := filepath.IsAbs(path)
	if !isAbs {
		err := errors.New("file path must be absolute path")
		return false, err
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func CreateFile(fileName, fileContent string) error {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR|os.O_TRUNC , 0640)
	if err != nil {
		return err
	}
	_, err = file.WriteString(fileContent)
	if err != nil {
		return err
	}
	return nil
}

func RenameFile(oldPath, newPath string) error {
	isExist, err := CheckFileExists(oldPath)
	if err != nil  {
		return err
	}
	if !isExist {
		return os.ErrNotExist
	}
	err = os.Rename(oldPath, newPath)
	if err != nil {
		return err
	}
	return nil
}

func ReadFileContent(path string) ([]byte, error) {
	isExist, err := CheckFileExists(path)
	if err != nil  {
		return nil, err
	}
	if !isExist {
		return nil, os.ErrNotExist
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// ValidateFileContent validates the file content by hash code
func ValidateFileContent(oldFileContent, newFileContent string) bool {
	if oldFileContent == "" && newFileContent == "" {
		return true
	}
	oldHash := hashContent([]byte(oldFileContent))
	newHash := hashContent([]byte(newFileContent))

	return oldHash == newHash
}

func hashContent(fileContent []byte) string {
	digest := sha256.Sum256(fileContent)
	return hex.EncodeToString(digest[:])
}