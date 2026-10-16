package workerd

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/VaalaCat/frp-panel/pb"
	"github.com/sourcegraph/conc"
)

func TestRunWorker(t *testing.T) {
	workerdBinPath := os.Getenv("FRP_PANEL_WORKERD_BIN")
	if workerdBinPath == "" {
		var err error
		workerdBinPath, err = exec.LookPath("workerd")
		if err != nil {
			t.Skip("workerd binary is not available; set FRP_PANEL_WORKERD_BIN to run this integration test")
		}
	}

	workerdCWD := t.TempDir()
	workerID := "test"

	c := context.Background()
	defaultWorker := &pb.Worker{WorkerId: &workerID}
	FillWorkerValue(defaultWorker, 1)

	if err := GenCapnpConfig(c, workerdCWD, &pb.WorkerList{Workers: []*pb.Worker{defaultWorker}}); err != nil {
		t.Fatalf("generate workerd config: %v", err)
	}

	var wg conc.WaitGroup

	wg.Go(func() {
		time.Sleep(10 * time.Second)
	})

	if err := WriteWorkerCodeToFile(c, defaultWorker, workerdCWD); err != nil {
		t.Fatalf("write worker code: %v", err)
	}

	runner := NewExecManager(workerdBinPath,
		[]string{"serve", "--watch", "--verbose"})
	runner.RunCmd(workerID, WorkerCWDPath(c, defaultWorker, workerdCWD),
		[]string{ConfigFilePath(c, defaultWorker, workerdCWD)})

	defer runner.ExitAllCmd()

	wg.Wait()
}
