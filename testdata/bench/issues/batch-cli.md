Add a --batch flag to `ohmylaya ask`

I call ask from a CI job for hundreds of inputs. Starting a process per input is slow; a mode that reads JSON lines and answers each one would help a lot.
