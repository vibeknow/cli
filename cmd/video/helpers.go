package video

import (
	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/cmdutil"
)

func newFiglensClient() (*figlens.Client, error) {
	_, url, tp, err := cmdutil.Default().Service("figlens")
	if err != nil {
		return nil, err
	}
	return figlens.New(url, tp), nil
}
