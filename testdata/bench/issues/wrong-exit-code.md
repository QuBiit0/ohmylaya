ohmylaya ask exits 0 when the tool returns an error

Passing an invalid JSON input prints an error to stderr but the exit code is 0, so my CI step passes when it should fail.
