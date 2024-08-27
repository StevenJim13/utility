package rotate

import (
	"bytes"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stkali/utility/errors"
	"github.com/stkali/utility/lib"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -package rotate -destination mock_WriteCloseStator_test.go github.com/stkali/utility/rotate WriteCloseStator

func TestNewFile(t *testing.T) {
	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	t.Run("default", func(t *testing.T) {
		name := lib.RandString(6)
		ext := "." + lib.RandString(2)
		filename := name + ext
		testFile := filepath.Join(testDir, filename)
		f, err := NewFile(testFile, nil)
		require.NoError(t, err)
		defer f.Close()

		require.Equal(t, testFile, f.fullPath)
		require.Equal(t, filename, f.filename)
		require.Equal(t, ext, f.ext)
		require.Equal(t, name, f.name)
		require.Equal(t, RotatingFilePrefix+name+"-", f.rotatingFilePrefix)
		require.Equal(t, SizeRotate|DurationRotate, f.mode)
		require.Equal(t, defaultMaxSize, f.option.MaxSize)
		require.Equal(t, defaultDuration, f.option.Duration)
		require.Equal(t, defaultBackups, f.option.Backups)
		require.Equal(t, int64(0), f.Used())
		require.Equal(t, false, f.cleaning.Load())
	})
	t.Run("not specify file", func(t *testing.T) {
		f, err := NewFile("", nil)
		require.Equal(t, err, NotSpecifyFileError)
		require.Nil(t, f)
	})
	t.Run("empty option", func(t *testing.T) {
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{})
		require.ErrorIs(t, err, NotRotateError)
		require.Nil(t, f)
	})
	t.Run("duration rotate", func(t *testing.T) {
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{Duration: time.Hour * 24})
		require.NoError(t, err)
		defer f.Close()

		require.Equal(t, testFile, f.fullPath)
		require.Equal(t, DurationRotate, f.mode)
	})
	t.Run("size rotate", func(t *testing.T) {
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 1 << 30})
		require.NoError(t, err)
		defer f.Close()

		require.Equal(t, testFile, f.fullPath)
		require.Equal(t, SizeRotate, f.mode)

	})
	t.Run("mode permission", func(t *testing.T) {
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		opt := getDefaultOption()
		opt.ModePerm = 0o177
		f, err := NewFile(testFile, opt)
		require.ErrorIs(t, err, ModePermissionError)
		require.Nil(t, f)

	})
	t.Run("mode error", func(t *testing.T) {
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{})
		require.ErrorIs(t, err, NotRotateError)
		require.Nil(t, f)
	})
}

func TestFileString(t *testing.T) {
	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	filename := lib.RandString(6) + ".rot"
	testFile := filepath.Join(testDir, filename)
	f, err := NewFile(testFile, nil)
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, filename, f.String())
}

func TestSetDuration(t *testing.T) {
	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
	f, err := NewFile(testFile, &Option{Duration: time.Hour * 24, MaxSize: defaultMaxSize})
	require.NoError(t, err)
	defer f.Close()

	t.Run("successfully set duration", func(t *testing.T) {
		err := f.SetDuration(time.Hour * 24 * 7)
		require.NoError(t, err)
		require.Equal(t, time.Hour*24*7, f.option.Duration)
	})

	t.Run("set duration to 0", func(t *testing.T) {

		err := f.SetDuration(0)
		require.NoError(t, err)
		require.Equal(t, time.Duration(0), f.option.Duration)
		require.True(t, f.mode&DurationRotate == 0)

	})

	t.Run("set duration to -1", func(t *testing.T) {
		err := f.SetDuration(-1)
		require.NoError(t, err)
		require.Equal(t, time.Duration(-1), f.option.Duration)
		require.True(t, f.mode&DurationRotate == 0)
	})

	t.Run("set duration < time.Hour", func(t *testing.T) {
		buf := &bytes.Buffer{}
		errors.SetWarningOutput(buf)
		err := f.SetDuration(time.Minute * 59)
		require.NoError(t, err)
		require.Equal(t, time.Minute*59, f.option.Duration)
		require.True(t, f.mode&DurationRotate != 0)
		warningText := buf.String()
		require.Contains(t, warningText, "is less than 1 hour")
	})

	t.Run("set duration failed", func(t *testing.T) {
		f.mode = 0
		err := f.SetDuration(0)
		fmt.Println(err)
		require.ErrorIs(t, err, NotRotateError)
	})
}

func TestSetMaxSize(t *testing.T) {
	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
	f, err := NewFile(testFile, &Option{MaxSize: 1 << 28, Duration: time.Hour * 24})
	require.NoError(t, err)
	defer f.Close()

	t.Run("successfully set MaxSize", func(t *testing.T) {
		err := f.SetMaxSize(1 << 30)
		require.NoError(t, err)
		require.Equal(t, int64(1<<30), f.option.MaxSize)
	})

	t.Run("set MaxSize to 0", func(t *testing.T) {

		err := f.SetMaxSize(0)
		require.NoError(t, err)
		require.Equal(t, int64(0), f.option.MaxSize)
		require.True(t, f.mode&SizeRotate == 0)
	})

	t.Run("set duration to -1", func(t *testing.T) {
		f.SetMaxSize(-1)
		require.Equal(t, int64(-1), f.option.MaxSize)
		require.True(t, f.mode&SizeRotate == 0)
	})

	t.Run("set MaxSize failed", func(t *testing.T) {
		f.mode = 0
		err := f.SetMaxSize(0)
		fmt.Println(err)
		require.ErrorIs(t, err, NotRotateError)
	})
}

func TestSetBackups(t *testing.T) {
	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
	f, err := NewFile(testFile, &Option{MaxSize: 1 << 28, Backups: 10})
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, 10, f.option.Backups)

	f.SetBackups(5)
	require.Equal(t, 5, f.option.Backups)

	f.SetBackups(-5)
	require.Equal(t, -5, f.option.Backups)

	buf := &bytes.Buffer{}
	errors.SetWarningOutput(buf)
	f.SetBackups(0)
	require.Equal(t, 0, f.option.Backups)
	require.Contains(t, buf.String(), "Backups is set to 0")

}

func TestModePerm(t *testing.T) {
	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
	f, err := NewFile(testFile, &Option{MaxSize: 1 << 28, Duration: time.Hour * 24, ModePerm: 0o777})
	require.NoError(t, err)
	defer f.Close()

	t.Run("default", func(t *testing.T) {
		err := f.SetModePerm(defaultModePerm)
		require.NoError(t, err)
		require.Equal(t, fs.FileMode(defaultModePerm), f.option.ModePerm)
	})

	t.Run("too tiny", func(t *testing.T) {
		err := f.SetModePerm(0o000)
		require.ErrorIs(t, err, ModePermissionError)
		err = f.SetModePerm(0o177)
		require.ErrorIs(t, err, ModePermissionError)
	})
}

func TestWriteStringAndWrite(t *testing.T) {
	t.Run("successfully write string and write", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 1 << 10, Duration: time.Hour * 24})
		require.NoError(t, err)
		defer f.Close()
		n, err := f.WriteString("hello")
		require.Equal(t, 5, n)
		require.NoError(t, err)
		n, err = f.Write(nil)
		require.Equal(t, 0, n)
		require.NoError(t, err)
		n, err = f.Write([]byte("world"))
		require.Equal(t, 5, n)
		require.NoError(t, err)
	})
	t.Run("write string failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		w := NewMockWriteCloseStator(ctrl)
		retErr := errors.Error("write string failed")
		w.EXPECT().Write(gomock.Any()).Return(0, retErr)
		f, err := NewFile("test", nil)
		require.NoError(t, err)
		err = f.SetDuration(-1)
		require.NoError(t, err)
		f.recorder = w
		n, err := f.WriteString("hello")
		require.Equal(t, 0, n)
		require.ErrorIs(t, err, retErr)
	})
}

func TestRoll(t *testing.T) {
	t.Run("backup < 0", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 10, Backups: -1, Duration: time.Hour * 24})
		require.NoError(t, err)
		defer f.Close()
		number := rand.Intn(10) + 5
		for i := 0; i < number; i++ {
			n, err := f.WriteString(lib.RandString(12))
			require.NoError(t, err)
			require.Equal(t, 12, n)
		}
		files, err := f.BackupFiles()

		require.Equal(t, number-1, len(files))
		err = f.roll(time.Now())
		require.NoError(t, err)
		files, err = f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, number, len(files))
	})
	t.Run("backup == 0", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 10, Backups: 30, Duration: time.Hour * 24})
		f.SetBackups(0)
		require.NoError(t, err)
		defer f.Close()
		number := rand.Intn(10) + 5
		for i := 0; i < number; i++ {
			n, err := f.WriteString(lib.RandString(12))
			require.NoError(t, err)
			require.Equal(t, 12, n)
		}
		files, err := f.BackupFiles()
		require.Equal(t, 0, len(files))
	})
	t.Run("no file", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 10, Backups: 30, Duration: time.Hour * 24})
		require.NoError(t, err)
		defer f.Close()
		n, err := f.WriteString(lib.RandString(12))
		require.NoError(t, err)
		require.Equal(t, 12, n)

		buf := &bytes.Buffer{}
		errors.SetWarningOutput(buf)
		err = os.RemoveAll(f.fullPath)
		require.NoError(t, err)
		err = f.roll(time.Now())
		require.NoError(t, err)
		require.Contains(t, buf.String(), "no such file or directory")
	})

	t.Run("roll failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		w := NewMockWriteCloseStator(ctrl)
		retErr := errors.Error("roll failed")
		w.EXPECT().Close().Return(retErr)
		f, err := NewFile("test", nil)
		require.NoError(t, err)
		f.recorder = w
		err = f.roll(time.Now())
		require.ErrorIs(t, err, retErr)
	})

}

func TestSizeRotate(t *testing.T) {
	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
	f, err := NewFile(testFile, &Option{MaxSize: 10})
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, SizeRotate, f.mode)
	require.Equal(t, int64(0), f.Used())

	n, err := f.WriteString("hello")
	require.Equal(t, 5, n)
	require.NoError(t, err)
	require.Equal(t, int64(5), f.Used())

	files, err := f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 0, len(files))

	n, err = f.WriteString("world!\n")
	require.Equal(t, 7, n)
	require.NoError(t, err)
	require.Equal(t, int64(7+5), f.Used())
	files, err = f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 0, len(files))

	n, err = f.WriteString("1")
	require.Equal(t, 1, n)
	require.NoError(t, err)
	require.Equal(t, int64(1), f.Used())
	files, err = f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 1, len(files))

}

func TestCheck(t *testing.T) {

	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
	f, err := NewFile(testFile, &Option{MaxSize: 1 << 30})
	require.NoError(t, err)
	defer f.Close()

	// set permission to 000
	err = os.Chmod(f.path, 0o000)
	require.NoError(t, err)
	n, err := f.WriteString("hello world!\n")
	require.ErrorIs(t, err, os.ErrPermission)
	require.Equal(t, 0, n)
	err = os.Chmod(f.path, 0o777)
	require.NoError(t, err)
	n, err = f.WriteString("hello world!\n")
	require.NoError(t, err)

	err = os.Remove(f.fullPath)
	require.NoError(t, err)

	n, err = f.WriteString(lib.RandString(12))
	require.NoError(t, err)
}

func TestDurationRotate(t *testing.T) {

	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
	f, err := NewFile(testFile, &Option{Duration: time.Second})
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, DurationRotate, f.mode)
	require.Equal(t, int64(0), f.Used())

	n, err := f.WriteString("hello")
	require.Equal(t, 5, n)
	require.NoError(t, err)
	require.Equal(t, int64(5), f.Used())

	files, err := f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 0, len(files))

	n, err = f.WriteString("world!\n")
	require.Equal(t, 7, n)
	require.NoError(t, err)
	require.Equal(t, int64(7+5), f.Used())
	files, err = f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 0, len(files))
	time.Sleep(1 * time.Second)

	n, err = f.WriteString("1")
	require.Equal(t, 1, n)
	require.NoError(t, err)
	require.Equal(t, int64(1), f.Used())
	files, err = f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 1, len(files))

}

func TestDurationAndSizeRotate(t *testing.T) {

	testDir := t.TempDir()
	defer os.RemoveAll(testDir)
	testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
	f, err := NewFile(testFile, &Option{MaxSize: 10, Duration: time.Second})
	require.NoError(t, err)
	defer f.Close()
	require.Equal(t, SizeRotate|DurationRotate, f.mode)
	require.Equal(t, int64(0), f.Used())

	n, err := f.WriteString("hello")
	require.Equal(t, 5, n)
	require.NoError(t, err)
	require.Equal(t, int64(5), f.Used())

	files, err := f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 0, len(files))

	n, err = f.WriteString("world!\n")
	require.Equal(t, 7, n)
	require.NoError(t, err)
	require.Equal(t, int64(7+5), f.Used())
	files, err = f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 0, len(files))

	n, err = f.WriteString("1")
	require.Equal(t, 1, n)
	require.NoError(t, err)
	require.Equal(t, int64(1), f.Used())
	files, err = f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 1, len(files))
	time.Sleep(1 * time.Second)

	n, err = f.WriteString("2")
	require.Equal(t, 1, n)
	require.NoError(t, err)
	require.Equal(t, int64(1), f.Used())
	files, err = f.BackupFiles()
	require.NoError(t, err)
	require.Equal(t, 2, len(files))
}

func TestCleanBackups(t *testing.T) {

	t.Run("size mode", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 10, Backups: 10, CleanupBlock: true})
		require.NoError(t, err)
		defer f.Close()
		require.Equal(t, SizeRotate, f.mode)
		require.Equal(t, int64(0), f.Used())
		count := 6
		for i := 0; i < count; i++ {
			n, err := f.WriteString("hello world!\n")
			require.Equal(t, 13, n)
			require.NoError(t, err)
		}
		files, err := f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, count-1, len(files))

		f.SetBackups(3)
		files, err = f.BackupFiles()
		f.SetBlock(true)
		err = f.CleanBackups()
		require.NoError(t, err)
		files, err = f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, 3, len(files))
	})

	t.Run("duration mode", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{Duration: time.Millisecond * 100, Backups: 10, CleanupBlock: true})
		require.NoError(t, err)
		defer f.Close()
		require.Equal(t, DurationRotate, f.mode)
		require.Equal(t, int64(0), f.Used())
		n, err := f.WriteString("hello world!\n")
		require.Equal(t, 13, n)
		require.NoError(t, err)
		time.Sleep(time.Millisecond * 110)
		n, err = f.WriteString("hello world!\n")
		require.Equal(t, 13, n)
		require.NoError(t, err)
		files, err := f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, 1, len(files))
	})

	t.Run("specify MaxAge", func(t *testing.T) {
		testDir := t.TempDir()
		//testDir := paths.ToAbsPath("./testdata/rotate")
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 10, Backups: -1, MaxAge: -1, CleanupBlock: true})
		require.NoError(t, err)
		defer f.Close()
		backupNumber := 3
		f.cleaning.Store(true)
		// generate backup files
		count := rand.Intn(10) + backupNumber
		for i := 0; i < count; i++ {
			n, err := f.WriteString("hello world!\n")
			require.Equal(t, 13, n)
			require.NoError(t, err)
		}
		files, err := f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, count-1, len(files))

		f.SetMaxAge(time.Microsecond * 10)
		time.Sleep(time.Microsecond * 20)
		f.cleaning.Store(false)
		err = f.CleanBackups()
		require.NoError(t, err)
		files, err = f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, 0, len(files))
	})

	t.Run("deleted rotating folder", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 10, Backups: 10, MaxAge: time.Hour, CleanupBlock: true})
		require.NoError(t, err)
		defer f.Close()
		number := 6
		for i := 0; i < number; i++ {
			n, err := f.WriteString("hello world!\n")
			require.Equal(t, 13, n)
			require.NoError(t, err)
		}
		files, err := f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, number-1, len(files))
		err = os.RemoveAll(f.path)
		require.NoError(t, err)
		err = f.cleanBackups()
		require.ErrorIs(t, err, os.ErrNotExist)

	})

	t.Run("no backup files", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 10, Backups: 10, MaxAge: time.Hour, CleanupBlock: true})
		require.NoError(t, err)
		defer f.Close()
		number := rand.Intn(10) + 5
		for i := 0; i < number; i++ {
			n, err := f.WriteString("hello world!\n")
			require.Equal(t, 13, n)
			require.NoError(t, err)
		}
		files, err := f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, number-1, len(files))
		f.SetBackups(0)
		err = f.cleanBackups()
		require.NoError(t, err)
		files, err = f.BackupFiles()
		require.NoError(t, err)
		require.Equal(t, 0, len(files))
	})
}

func TestFileNextBackupFile(t *testing.T) {
	now := time.Now()
	after := now.Add(time.Hour)
	before := now.Add(-time.Hour)
	file, err := NewFile("test", nil)
	require.NoError(t, err)
	defer file.Close()

	beforeFilename := file.NextBackupFile(before)
	nowFilename := file.NextBackupFile(now)
	afterFilename := file.NextBackupFile(after)

	beforeTimeStr := beforeFilename[len(file.rotatingFilePrefix) : len(beforeFilename)-len(file.ext)-saultWidth]
	nowTimeStr := nowFilename[len(file.rotatingFilePrefix) : len(nowFilename)-len(file.ext)-saultWidth]
	afterTimeStr := afterFilename[len(file.rotatingFilePrefix) : len(afterFilename)-len(file.ext)-saultWidth]

	beforeTime, err := time.Parse(file.option.BackupTimeFormat, beforeTimeStr)
	require.NoError(t, err)
	require.True(t, before.Sub(beforeTime) <= time.Hour)

	nowTime, err := time.Parse(file.option.BackupTimeFormat, nowTimeStr)
	require.NoError(t, err)
	require.True(t, now.Sub(nowTime) <= time.Hour)

	afterTime, err := time.Parse(file.option.BackupTimeFormat, afterTimeStr)
	require.NoError(t, err)
	require.True(t, after.Sub(afterTime) <= time.Hour)
}

func TestDeleteBackupFile(t *testing.T) {
	file, err := NewFile("test", nil)
	require.NoError(t, err)
	defer file.Close()
	buf := &bytes.Buffer{}
	errors.SetWarningOutput(buf)
	err = file.deleteBackupFiles([]string{"test1", "test2", "test3"})
	require.NoError(t, err)
	require.Contains(t, buf.String(), "failed to remove backup file")
}

func TestClose(t *testing.T) {

	t.Run("success", func(t *testing.T) {
		testDir := t.TempDir()
		defer os.RemoveAll(testDir)
		testFile := filepath.Join(testDir, lib.RandString(6)+".rot")
		f, err := NewFile(testFile, &Option{MaxSize: 10, Duration: time.Hour * 24})
		require.NoError(t, err)
		require.Nil(t, f.ticker)
		err = f.check()
		require.NoError(t, err)
		require.NotNil(t, f.ticker)
		err = f.Close()
		require.NoError(t, err)
		require.Nil(t, f.ticker)
		require.Nil(t, f.recorder)
	})
	t.Run("failed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		recorder := NewMockWriteCloseStator(ctrl)
		err := fmt.Errorf("close error")
		recorder.EXPECT().Close().Return(err)
		file := File{
			recorder: recorder,
			option:   getDefaultOption(),
		}
		wrapperErr := file.Close()
		require.Error(t, err)
		require.ErrorIs(t, wrapperErr, err)
	})
}
