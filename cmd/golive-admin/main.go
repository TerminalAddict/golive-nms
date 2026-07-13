package main

import (
	"archive/tar"
	"context"
	"filippo.io/age"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: golive-admin backup|schedule|restore <file>")
	}
	switch os.Args[1] {
	case "backup":
		if _, err := backup(); err != nil {
			fatal(err.Error())
		}
	case "schedule":
		schedule()
	case "restore":
		if len(os.Args) != 3 {
			fatal("restore requires a backup file")
		}
		if err := restore(os.Args[2]); err != nil {
			fatal(err.Error())
		}
	default:
		fatal("unknown command")
	}
}
func backup() (string, error) {
	pass := os.Getenv("GOLIVE_BACKUP_PASSPHRASE")
	if len(pass) < 16 {
		return "", fmt.Errorf("GOLIVE_BACKUP_PASSPHRASE must be at least 16 characters")
	}
	if err := os.MkdirAll("/backups", 0700); err != nil {
		return "", err
	}
	name := filepath.Join("/backups", "golive-"+time.Now().UTC().Format("20060102T150405Z")+".tar.age")
	file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	recipient, err := age.NewScryptRecipient(pass)
	if err != nil {
		file.Close()
		return "", err
	}
	recipient.SetWorkFactor(18)
	encrypted, err := age.Encrypt(file, recipient)
	if err != nil {
		file.Close()
		return "", err
	}
	tw := tar.NewWriter(encrypted)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pg_dump", "--clean", "--if-exists", "--no-owner", "--no-privileges", databaseURL())
	pipe, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err == nil {
		err = writeEntry(tw, "database.sql", pipe, time.Now())
	}
	if waitErr := cmd.Wait(); err == nil {
		err = waitErr
	}
	if err == nil {
		_ = createSnapshot("http://victoriametrics:8428/snapshot/create")
		err = addTree(tw, "/metrics", "metrics")
	}
	if err == nil {
		err = addTree(tw, "/logs", "logs")
	}
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := encrypted.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(name)
		return "", err
	}
	prune()
	fmt.Println(name)
	return name, nil
}
func schedule() {
	interval := durationEnv("GOLIVE_BACKUP_INTERVAL", 24*time.Hour)
	for {
		select {
		case <-time.After(interval):
			if _, err := backup(); err != nil {
				fmt.Fprintln(os.Stderr, "backup:", err)
			}
		}
	}
}
func restore(name string) error {
	pass := os.Getenv("GOLIVE_BACKUP_PASSPHRASE")
	identity, err := age.NewScryptIdentity(pass)
	if err != nil {
		return err
	}
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	decrypted, err := age.Decrypt(file, identity)
	if err != nil {
		return err
	}
	tr := tar.NewReader(decrypted)
	sql, err := os.CreateTemp("", "golive-restore-*.sql")
	if err != nil {
		return err
	}
	defer os.Remove(sql.Name())
	defer sql.Close()
	for {
		header, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if header.Name == "database.sql" {
			if _, e = io.Copy(sql, tr); e != nil {
				return e
			}
			continue
		}
		var root, relative string
		if strings.HasPrefix(header.Name, "metrics/") {
			root = "/metrics"
			relative = strings.TrimPrefix(header.Name, "metrics/")
		} else if strings.HasPrefix(header.Name, "logs/") {
			root = "/logs"
			relative = strings.TrimPrefix(header.Name, "logs/")
		} else {
			continue
		}
		target := filepath.Join(root, filepath.Clean(relative))
		if !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe archive path")
		}
		if header.Typeflag == tar.TypeDir {
			if e = os.MkdirAll(target, os.FileMode(header.Mode)); e != nil {
				return e
			}
		} else if header.Typeflag == tar.TypeReg {
			if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
				return e
			}
			out, e := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if e != nil {
				return e
			}
			_, e = io.Copy(out, tr)
			out.Close()
			if e != nil {
				return e
			}
		}
	}
	if err = sql.Close(); err != nil {
		return err
	}
	cmd := exec.Command("psql", databaseURL(), "-v", "ON_ERROR_STOP=1", "-f", sql.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func addTree(tw *tar.Writer, root, prefix string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		name := filepath.ToSlash(filepath.Join(prefix, relative))
		header, e := tar.FileInfoHeader(info, "")
		if e != nil {
			return e
		}
		header.Name = name
		if e = tw.WriteHeader(header); e != nil {
			return e
		}
		if info.Mode().IsRegular() {
			f, e := os.Open(path)
			if e != nil {
				return e
			}
			_, e = io.Copy(tw, f)
			f.Close()
			return e
		}
		return nil
	})
}
func writeEntry(tw *tar.Writer, name string, reader io.Reader, modified time.Time) error {
	temp, err := os.CreateTemp("", "golive-pgdump-*.sql")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	size, err := io.Copy(temp, reader)
	if err != nil {
		return err
	}
	if _, err = temp.Seek(0, 0); err != nil {
		return err
	}
	if err = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: size, ModTime: modified}); err != nil {
		return err
	}
	_, err = io.Copy(tw, temp)
	return err
}
func databaseURL() string {
	if v := os.Getenv("GOLIVE_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://golive@postgres:5432/golive?sslmode=disable"
}
func createSnapshot(url string) error {
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("snapshot API returned %s", resp.Status)
	}
	return nil
}
func prune() {
	keep := 10
	if v, e := strconv.Atoi(os.Getenv("GOLIVE_BACKUP_KEEP")); e == nil && v > 0 {
		keep = v
	}
	files, _ := filepath.Glob("/backups/golive-*.tar.age")
	sort.Strings(files)
	for len(files) > keep {
		_ = os.Remove(files[0])
		files = files[1:]
	}
}
func durationEnv(key string, fallback time.Duration) time.Duration {
	if v, e := time.ParseDuration(os.Getenv(key)); e == nil && v > 0 {
		return v
	}
	return fallback
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
