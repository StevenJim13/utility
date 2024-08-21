package lib

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test2String(t *testing.T) {
	cases := []struct {
		Name   string
		Bytes  []byte
		Expect string
	}{
		{
			"empty",
			[]byte{},
			"",
		},
		{
			"integer",
			[]byte("1"),
			"1",
		},

		{
			"return",
			[]byte("\r"),
			"\r",
		},

		{
			"newline",
			[]byte("\n"),
			"\n",
		},
		{
			"other",
			[]byte("\r\n928176\tasljh\tt"),
			"\r\n928176\tasljh\tt",
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			require.Equal(t, c.Expect, ToString(c.Bytes))
		})
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			require.Equal(t, c.Bytes, ToBytes(c.Expect))
		})
	}
}

func TestSize2String(t *testing.T) {
	// 正常路径测试
	s, err := Size2String(1024)
	assert.NoError(t, err)
	assert.Equal(t, "1.00 KB", s, "Size2String 应该返回正确的字符串")

	// 边界测试
	s, err = Size2String(0)
	assert.NoError(t, err)
	assert.Equal(t, "0 B", s, "Size2String 应该处理零值")

	s, err = Size2String(-1)
	assert.Error(t, err)
	s, err = Size2String(1024 * 1024 * 1024 * 1024 * 1024 * 1024)
	assert.NoError(t, err)
	assert.Equal(t, "1.00 EB", s, "Size2String 应该处理极大值")
}

func TestString2Size(t *testing.T) {
	// 正常路径测试
	size, err := String2Size("1024 KB")
	assert.NoError(t, err)
	assert.Equal(t, 1024*KB, size, "String2Size 应该返回正确的字节大小")

	// 边界测试
	size, err = String2Size("0")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), size, "String2Size 应该处理零值")

	size, err = String2Size("-1")
	assert.Error(t, err, "String2Size 应该处理负值错误")

	size, err = String2Size("1 EB")
	assert.NoError(t, err)
	assert.Equal(t, EB, size, "String2Size 应该处理极大值")

	// 错误格式测试
	_, err = String2Size("1024ABC")
	assert.Error(t, err, "String2Size 应该处理错误格式")

	_, err = String2Size("ABC")
	assert.Error(t, err, "String2Size 应该处理错误格式")
}
