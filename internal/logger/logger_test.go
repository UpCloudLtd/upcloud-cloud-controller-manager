package logger_test

import (
	"flag"
	"strings"
	"testing"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/logger"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"
)

func TestKlog(t *testing.T) {
	t.Parallel()

	// NOTE: If this starts to hinder other tests, we can probably remove the whole test.
	defer klog.CaptureState().Restore()

	klog.InitFlags(nil)
	require.NoError(t, flag.Set("logtostderr", "false"))
	flag.Parse()

	var b strings.Builder

	klog.SetOutput(&b)
	log := logger.NewKlog()
	log.Infof("logger %s", "test")
	klog.Flush()

	require.True(t, strings.HasSuffix(b.String(), "logger test\n"))
}
