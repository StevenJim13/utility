package rotate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stkali/utility/errors"
	"github.com/stkali/utility/lib"
	"github.com/stkali/utility/paths"
)

const (
	SizeRotate = 1 << iota
	DurationRotate

	WriteMode                     = 0o200
	RotatingFilePrefix            = "rotating-"
	defaultBackups                = 30
	defaultModePerm               = 0o644
	defaultMaxAge                 = 30 * 24 * time.Hour
	defaultMaxSize          int64 = 1 << 28
	defaultDuration               = 24 * time.Hour
	defaultBackupTimeFormat       = "2006-01-02:15-04-05.0000"
	saultWidth                    = 6
)

var (
	// NotRotateError is an error that is returned when no specify MaxSize and Duration.
	NotRotateError       = errors.Error("no specify MaxSize or Duration")
	NotSpecifyFileError  = errors.Error("not specify rotating file")
	ModePermissionError  = errors.Error("invalid mode permission")
	InvalidBackupsError  = errors.Error("invalid backups")
	InvalidMaxAgeError   = errors.Error("invalid max age")
	InvalidDurationError = errors.Error("invalid duration")
	InvalidMaxSizeError  = errors.Error("invalid max size")
)

type Option struct {

	// Duration specifies the time interval after which a new file should be created.
	// <= 0 means no rotation based on time interval.
	// `SetDuration` will modify the rotation mode.
	Duration time.Duration

	// MaxSize defines the threshold size (in bytes) that triggers a file rotation.
	// A new log file is created the used space in the current log file exceeds MaxSize.
	// If this value is 0, the rotating file size is not limited and a new file is
	// created only when the duration interval is reached.
	// <= 0 means no rotation based on file size.
	// `SetMaxSize` will modify the rotation mode.
	MaxSize int64

	// Backups defines the maximum number of backup files that can be retained after rotation.
	// When this limit is reached, the oldest backup file will be deleted to make room for new ones.
	// = 0 means no backup files are retained.
	// < 0 the backup deletion strategy based on `Backups` will not work.
	Backups int

	// CleanupBlock specifies whether backup file cleanup should be performed synchronously
	// or asynchronously (default false).
	// If false, cleanup will be performed in a separate goroutine to avoid blocking the main logging thread.
	CleanupBlock bool

	// MaxAge defines the maximum age that a backup file can have before it is considered for cleanup.
	// Files older than this duration will be deleted during the cleanup process.
	// <=0 the backup deletion strategy based on `MaxAge` will not work.
	// `SetMaxAge` will modify the cleanup mode.
	MaxAge time.Duration

	// ModePerm specifies the default file permission bits used when creating new log files.
	// This ensures that the log files are created with the desired security settings.
	// default is 0o644.
	// `SetModePerm` will modify the file permission bits.
	ModePerm os.FileMode
	// TODO: 增加日志文件压缩功能
	// Compress bool
	// CompressLevel int

	// BackupTimeFormat specifies the time format used when creating backup files.
	BackupTimeFormat string
}

// validate checks the validity of the options specified.
func (o *Option) validate() error {
	if o.Backups == 0 {
		o.Backups = defaultBackups
	}
	if o.BackupTimeFormat == "" {
		o.BackupTimeFormat = defaultBackupTimeFormat
	}
	if o.ModePerm != 0 && o.ModePerm&WriteMode == 0 {
		return ModePermissionError
	} else if o.ModePerm == 0 {
		o.ModePerm = defaultModePerm
	}
	return nil
}

// getDefaultOption returns the default options for a rotating file.
func getDefaultOption() *Option {
	return &Option{
		Duration:     defaultDuration,
		MaxSize:      defaultMaxSize,
		Backups:      defaultBackups,
		CleanupBlock: false,
		MaxAge:       defaultMaxAge,
		ModePerm:     defaultModePerm,
	}
}

// func durationRotate(f *File) error {
// 	select {
// 	case now := <-f.ticker.C:
// 		return f.roll(now)
// 	default:
// 		return nil
// 	}
// }

// func sizeRotate(f *File) error {
// 	if f.used < f.option.MaxSize {
// 		return nil
// 	}
// 	return f.roll(time.Now())
// }

// func multiRotate(f *File) error {
// 	select {
// 	case now := <-f.ticker.C:
// 		return f.roll(now)
// 	default:
// 		return sizeRotate(f)
// 	}
// }

// File represents a rotating file that can be used to write data to.
// It implements the io.Writer interface.
type File struct {
	// recorder is the current file descriptor (io.Writer) that is being written to.
	// It represents the currently active log file.
	recorder io.Writer

	// option contains the configuration options for the log file.
	option *Option

	// tryRotate is a function that is called to determine whether a new file should be created
	// based on the current log file size or time interval.
	// tryRotate func(f *File) error

	// ticker is a pointer to a time.Timer that is used to schedule the next file rotation
	// based on the duration specified. When the timer expires, a new file is created
	// and the timer is reset for the next rotation.
	ticker *time.Ticker

	// mtx is a mutex that ensures thread-safe access to the struct's fields and methods.
	// It prevents data races and ensures that log rotation and writing operations are synchronized.
	mtx sync.Mutex

	// cleaning (using an underscore prefix to avoid accidental use as a public field)
	// is an atomic.Bool that indicates whether a garbage collection (cleanup) task is currently being executed.
	// This allows for safe and efficient cleanup of old backup files.
	cleaning atomic.Bool

	// used tracks the amount of space already used in the current log file (in bytes).
	// This value increases as log data is written and resets to 0 when the size threshold is reached.
	used int64

	// mode is a bitmask that indicates which rotation mode is being used.
	// It can be either SizeRotate or DurationRotate or both.
	mode int

	// fullPath is the full path of the rotating file.
	fullPath string

	// path is the full path of the rotating file.
	path string

	// filename is the name of the rotating file with extension.
	filename string

	// name is the name of the rotating file without extension.
	name string

	// ext is the extension of the rotating file.
	ext string

	rotatingFilePrefix string
}

// NewFile creates a new rotating log file with the specified options.
// The returned File object implements the io.Writer interface and can be used
// to write log data to the log file.
func NewFile(file string, option *Option) (*File, error) {

	if file == "" {
		return nil, NotSpecifyFileError
	}

	f := &File{}
	f.fullPath = paths.ToAbsPath(file)
	f.path, f.name, f.ext = paths.SplitWithExt(f.fullPath)
	f.filename = f.name + f.ext
	f.rotatingFilePrefix = fmt.Sprintf("%s%s-", RotatingFilePrefix, f.name)

	// set option
	if option == nil {
		option = getDefaultOption()
	}
	err := option.validate()
	if err != nil {
		return nil, err
	}
	f.option = option
	if option.Duration > 0 {
		f.mode |= DurationRotate
	}
	if option.MaxSize > 0 {
		f.mode |= SizeRotate
	}
	// f.tryRotate, err = matchRotate(f.mode)
	if err != nil {
		return nil, errors.Newf("failed to create File, err: %s", err)
	}
	return f, nil
}

// SetDuration set the time interval for rotating log files.
func (f *File) SetDuration(duration time.Duration) error {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	mode := f.mode
	if duration <= 0 {
		mode &= ^DurationRotate
	} else {
		mode |= DurationRotate
	}
	// tryRotate, err := matchRotate(mode)
	// if err != nil {
	// 	return errors.Newf("failed to set duration, err: %s", err)
	// }
	f.mode = mode
	f.option.Duration = duration
	// f.tryRotate = tryRotate
	if duration < time.Hour {
		errors.Warningf("duration:%s is less than 1 hour, it may make too many backup files", duration)
	}
	return nil
}

// SetMaxSize set the maximum size of a log file before it is rotated.
func (f *File) SetMaxSize(size int64) error {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	mode := f.mode
	if size <= 0 {
		mode &= ^SizeRotate
	} else {
		mode |= SizeRotate
	}
	// rotate, err := matchRotate(mode)
	// if err != nil {
	// 	return errors.Newf("failed to set MaxSize, err: %s", err)
	// }
	f.mode = mode
	f.option.MaxSize = size
	// f.tryRotate = rotate
	if f.option.MaxSize < 1<<22 {
		errors.Warningf("MaxAge:%s is less than 4M, it may make too many backup files", f.option.MaxAge)
	}
	return nil
}

// SetBackups set the maximum number of backup files that can be retained after rotation.
func (f *File) SetBackups(number int) {
	if number == 0 {
		errors.Warningf("Backups is set to 0, no backup files will be retained")
	}
	f.option.Backups = number
}

// SetMaxAge set the maximum age that a backup file can have before it is considered for cleanup.
func (f *File) SetMaxAge(age time.Duration) {
	f.option.MaxAge = age
}

// SetBlock set the cleanup block option.
// If block is true, the cleanup will be performed in the current goroutine, otherwise it will be performed
// in a separate goroutine to avoid blocking the main writing goroutine.
func (f *File) SetBlock(block bool) {
	f.option.CleanupBlock = block
}

// SetModePerm set the default file permission bits used when creating new log files.
func (f *File) SetModePerm(perm os.FileMode) error {
	if perm&WriteMode == 0 {
		return ModePermissionError
	}
	f.option.ModePerm = perm
	return nil
}

// Used returns the amount of space already used in the current log file (in bytes).
func (f *File) Used() int64 {
	return f.used
}

func (f *File) String() string {
	return f.filename
}

// Write writes the specified data to the rotating file.
func (f *File) Write(b []byte) (int, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	// write
	n, err := f.recorder.Write(b)
	if err != nil {
		return n, errors.Newf("failed to write %s to file: %s, err: %s", lib.ToString(b), f.filename, err)
	}
	// update used
	f.used += int64(n)
	if f.used > f.option.MaxSize {
		if err = f.rotate(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// WriteString writes the specified string to the rotating file.
func (f *File) WriteString(s string) (int, error) {
	return f.Write(lib.ToBytes(s))
}

// check checks the status of the log file, including whether a new file should be created
// based on the current log file size or time interval. If a new file is created,
// the current file descriptor is closed and a new file descriptor is opened.
func (f *File) check() error {
	if f.recorder == nil {
		fd, err := paths.MakeFile(f.fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, f.option.ModePerm)
		if err != nil {
			return err
		}
		f.recorder = fd
		f.used = 0
		// check duration
		// err = f.setFD(fd)
		// if err != nil {
		// 	return err
		// }
		// return f.CleanBackups()
	}
	// if f.mode&SizeRotate != 0 && f.option.MaxSize > 0 && f.used >= f.option.MaxSize {
	// 	return f.roll(time.Now())
	// }
	return nil
}

type Stator interface {
	Stat() (os.FileInfo, error)
}

// WriteCloseStator is a file descriptor that implements both io.Writer, Stator and io.Closer interfaces.
// For mock testing.
type WriteCloseStator interface {
	io.WriteCloser
	Stator
}

func (f *File) setFD(fd io.Writer) error {
	f.recorder = fd
	f.used = 0
	// check duration
	if f.mode&DurationRotate != 0 {
		if f.ticker == nil {
			f.ticker = time.NewTicker(f.option.Duration)
		}
	}
	// check size
	if f.mode&SizeRotate != 0 {
		if fileDesc, ok := fd.(Stator); ok {
			stat, err := fileDesc.Stat()
			if err != nil {
				return errors.Newf("failed to get fd info, err: %s", err)
			}
			f.used = stat.Size()
		}
	}
	return nil
}

// rotate creates a new log file and closes the current file descriptor.
// It also performs backup file cleanup if necessary.
func (f *File) rotate() error {

	err := f.close()
	if err != nil {
		return err
	}

	if f.option.Backups != 0 {
		// backup >= 1
		backupFilename := f.NextBackupFile(time.Now())
		backupFile := filepath.Join(f.path, backupFilename)
		if paths.IsExisted(backupFilename) {
			return f.rotate()
		}
		err = os.Rename(f.fullPath, backupFile)
		if err != nil {
			if os.IsNotExist(err) {
				errors.Warningf("failed to backup file: %q, err: %s", backupFile, err)
				return nil
			}
		}
	}
	fd, err := paths.MakeFile(f.fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.option.ModePerm)
	if err != nil {
		return err
	}
	if err = f.setFD(fd); err != nil {
		return err
	}
	if f.option.Backups > 0 {
		return f.CleanBackups()
	}
	return nil
}

// NextBackupFile returns the name of the next backup file based on the current time.
func (f *File) NextBackupFile(now time.Time) string {
	sb := strings.Builder{}
	timeString := now.Format(f.option.BackupTimeFormat)
	salt := lib.RandString(saultWidth)
	sb.Grow(len(f.rotatingFilePrefix) + len(timeString) + len(salt) + len(f.ext) + 1)
	sb.WriteString(f.rotatingFilePrefix)
	sb.WriteString(timeString)
	sb.WriteString(salt)
	sb.WriteString(f.ext)
	return sb.String()
}

// CleanBackups performs garbage collection (cleanup) of old backup files.
// It deletes the oldest backup files until the maximum number of backup files is reached.
// It is safe to call this method multiple times concurrently.
// If the CleanupBlock option is set to false(default false), the cleanup will be performed in
// a separate goroutine to avoid blocking the main writing goroutine, otherwise it will be performed
// in the current goroutine.
func (f *File) CleanBackups() error {
	// existed a running cleanup goroutine
	if !f.cleaning.CompareAndSwap(false, true) {
		return nil
	}
	// block the goroutine until the clean finished
	if f.option.CleanupBlock {
		defer f.cleaning.Store(false)
		return f.cleanBackups()
	}
	// start a new goroutine to clean backups
	go func() {
		defer f.cleaning.Store(false)
		errors.Warning(f.cleanBackups())
	}()
	return nil
}

func (f *File) cleanBackups() error {
	backups, err := f.BackupFiles()
	if err != nil {
		return err
	}
	length := len(backups)
	if length == 0 {
		return nil
	}
	if f.option.Backups == 0 {
		return f.deleteBackupFiles(backups)
	}
	// sort backups by name(create time)
	sort.Strings(backups)
	deleteIndex := 0
	if f.option.Backups > 0 {
		if left := length - f.option.Backups; left > 0 {
			deleteIndex = left
		}
	}

	if f.option.MaxAge > 0 {
		width := len(f.rotatingFilePrefix) + len(f.option.BackupTimeFormat)
		limit := f.NextBackupFile(time.Now().Add(-f.option.MaxAge))[:width]
		index := slices.IndexFunc(backups, func(s string) bool {
			return s[:width] >= limit
		})
		// if the limit file is not found, all backups are older than the limit, so we can delete all backups
		if index == -1 {
			deleteIndex = length
		} else {
			deleteIndex = index
		}
	}
	// delete backups
	if deleteIndex > 0 {
		return f.deleteBackupFiles(backups[:deleteIndex])
	}
	return nil
}

func (f *File) BackupFiles() ([]string, error) {
	files, err := os.ReadDir(f.path)
	if err != nil {
		return nil, errors.Newf("failed to read directory: %s, err: %s", f.path, err)
	}
	backups := make([]string, 0, len(files))
	for _, file := range files {
		if !file.IsDir() && f.IsBackupFile(file.Name()) {
			backups = append(backups, file.Name())
		}
	}
	return backups, nil
}

// IsBackupFile returns true if the specified file is a backup file of the current log file.
func (f *File) IsBackupFile(file string) bool {
	return strings.HasPrefix(file, f.rotatingFilePrefix) && strings.HasSuffix(file, f.ext)
}

func (f *File) deleteBackupFiles(files []string) error {
	for _, file := range files {
		filename := filepath.Join(f.path, file)
		if err := os.Remove(filename); err != nil {
			errors.Warningf("failed to remove backup file: %q, err: %s", filename, err)
		}
	}
	return nil
}

// Close closes the log file and releases any associated resources.
func (f *File) Close() error {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	// wait for the cleanup goroutine to finish
	for !f.option.CleanupBlock && f.cleaning.Load() {
	}
	err := f.close()
	if err != nil {
		return err
	}
	if f.ticker != nil {
		f.ticker.Stop()
		f.ticker = nil
	}
	return nil
}

// Close closes the rotate file.
func (f *File) close() error {
	if closer, ok := f.recorder.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return errors.Newf("failed to close recoder: %s, err: %s", f.recorder, err)
		}
	}
	f.recorder = nil
	f.used = 0
	return nil
}

//func NewSizeRotateFile(file string, size int64) (*File, error) {
//	option := Option{
//		MaxSize: size,
//	}
//	return NewFile(file, &option)
//}
//
//func NewDurationRotateFile(file string, duration time.Duration) (*File, error) {
//	option := &Option{
//		Duration: duration,
//	}
//	return NewFile(file, option)
//}
//
//func NewRotateFile(file string, size int64, duration time.Duration) (*File, error) {
//	option := getDefaultOption()
//	option.MaxSize = size
//	option.Duration = duration
//	return NewFile(file, option)
//}

type Simple struct {
	writer   io.Writer
	maxSize  int64
	used     int64
	filename string
}

func (s *Simple) Write(b []byte) (int, error) {
	n, err := s.writer.Write(b)
	if err != nil {
		return n, errors.Newf("failed to write %s to file: %s, err: %s", lib.ToString(b), s.filename, err)
	}
	s.used += int64(n)
	if s.used > s.maxSize {
		//
	}
	return n, nil
}



func (s *Simple) WriteString(t string) (int, error) {
	return s.Write(lib.ToBytes(t))
}

func (s *Simple) Close() error {
	if closer, ok := s.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
