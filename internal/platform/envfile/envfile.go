// Package envfile reads the deliberately small KEY=VALUE subset used by the
// native systemd services. It does not perform shell expansion or substitution.
package envfile

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	MaximumFileBytes = 1 << 20
	MaximumLineBytes = 64 << 10
)

var keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// LoadProcess loads the optional project .env file without overriding values
// already supplied by the operating system. WERK_ENV_FILE makes another path
// explicit and therefore required.
func LoadProcess() error {
	path := strings.TrimSpace(os.Getenv("WERK_ENV_FILE"))
	required := path != ""
	if path == "" {
		path = ".env"
	}
	values, err := LoadSecure(path)
	if err != nil {
		if !required && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := ValidateConfigured(values); err != nil {
		return err
	}
	for key, value := range values {
		if _, configured := os.LookupEnv(key); configured {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("apply environment value %s: %w", key, err)
		}
	}
	return nil
}

// ValidateConfigured rejects template markers without including their values in
// the error. A copied example must never look like a runnable installation.
func ValidateConfigured(values map[string]string) error {
	for key, value := range values {
		if strings.Contains(strings.ToUpper(value), "CHANGE_ME") {
			return fmt.Errorf("environment value %s still contains CHANGE_ME", key)
		}
	}
	return nil
}

// LoadSecure reads a service environment file and rejects permissions that
// expose secrets to other users or permit group modification.
func LoadSecure(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open environment file: %w", err)
	}
	defer file.Close()
	information, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect environment file: %w", err)
	}
	if !information.Mode().IsRegular() {
		return nil, errors.New("environment file must be a regular file")
	}
	permissions := information.Mode().Perm()
	if runtime.GOOS != "windows" && permissions&0o037 != 0 {
		return nil, fmt.Errorf("environment file permissions %04o must not grant group write/execute or any access to others", permissions)
	}
	return Parse(io.LimitReader(file, MaximumFileBytes+1))
}

func Parse(reader io.Reader) (map[string]string, error) {
	contents, err := io.ReadAll(io.LimitReader(reader, MaximumFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read environment file: %w", err)
	}
	if len(contents) > MaximumFileBytes {
		return nil, fmt.Errorf("environment file exceeds %d bytes", MaximumFileBytes)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	scanner.Buffer(make([]byte, 4096), MaximumLineBytes)
	values := make(map[string]string)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rawValue, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !keyPattern.MatchString(key) {
			return nil, fmt.Errorf("environment line %d has an invalid key assignment", lineNumber)
		}
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("environment line %d repeats %s", lineNumber, key)
		}
		value, err := parseValue(strings.TrimSpace(rawValue))
		if err != nil {
			return nil, fmt.Errorf("environment line %d: %w", lineNumber, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read environment file: %w", err)
	}
	return values, nil
}

func parseValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("unterminated single-quoted value")
		}
		return value[1 : len(value)-1], nil
	}
	if value[0] == '"' {
		parsed, err := strconv.Unquote(value)
		if err != nil {
			return "", errors.New("invalid double-quoted value")
		}
		return parsed, nil
	}
	if strings.ContainsAny(value, "\"'`$\\") {
		return "", errors.New("unquoted value contains shell syntax; quote it explicitly")
	}
	return value, nil
}
