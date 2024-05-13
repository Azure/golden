package golden

import (
	"github.com/prashantv/gostub"
	"github.com/spf13/afero"
)

var testFsFactory = func() afero.Fs {
	return configFs
}

type testBase struct {
	fs   afero.Fs
	stub *gostub.Stubs
}

func newTestBase() *testBase {
	t := new(testBase)
	t.fs = afero.NewMemMapFs()
	t.stub = gostub.Stub(&configFs, t.fs)
	return t
}

func (t *testBase) teardown() {
	t.stub.Reset()
}

func (t *testBase) dummyFsWithFiles(files map[string]string) {
	for name, content := range files {
		n := name
		_ = afero.WriteFile(t.fs, n, []byte(content), 0644)
	}
}
