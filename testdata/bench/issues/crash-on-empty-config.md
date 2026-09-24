ohmylaya doctor panics when config.toml is empty

Steps: create an empty ~/.ohmylaya/config.toml and run `ohmylaya doctor`.
Expected: a message saying the file is invalid. Actual: panic: runtime error: invalid memory address or nil pointer dereference in config.Load.
