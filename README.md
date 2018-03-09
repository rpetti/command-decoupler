# command-decoupler
## What is this thing?
This is a program designed for windows that allows specified sub-commands to be executed in a different process tree.

For example, if you are running a process that calls regsvr32, the normal execution will look like this:
```
cmd.exe - process.exe
               |
                - regsvr32.exe
```

With command-decoupler, it will look like this
```
cmd.exe - command-decoupler.exe
                |
                 - process.exe
                |
                 - regsvr32.exe
```

## But... Why?
This program, in most situations, is completely useless. My use-case for this was spurred on by a bug in HPE's Fortify which caused some sub processes be behave incorrectly when wrapped by it's sourceanalyzer command. Full blogpost is [here](http://robpetti.com/fortify-breaks-regsvr32/).

For example, if I had a C++ project that was configured to register a COM or OCX object, the following would fail:
```
sourceanalyzer.exe -b myid msbuild myproject\myproject.sln /p:Configuration=Release /p:Platform=Win32
```
The process tree looks something like this:
```
cmd.exe - sourceanalyzer.exe
                |
                 - msbuild
                        |
                         - regsvr32.exe (Fails to load OCX)
```

Using the command-decoupler to move the regsvr32 command to the outside of the Fortify process tree mysteriously works fine. The process tree would instead look like this:
```
cmd.exe - command-decoupler.exe
                |
                 - sourceanalyzer.exe
                |           |
                |            - msbuild.exe
                 - regsvr32.exe
```

## Ok... How does this work?
Command-decoupler performs the following actions:
1. Creates a named pipe, and starts a goroutine to monitor it for execution requests.
2. For each -cmd specified, it creates a copy of itself with that name and places it into a temp folder (overridable by -path)
3. It adds the temp folder (or folder specified by -path) to the PATH env var
4. It starts the specified subcommand
5. Any time the subcommand calls one of the masqerading binaries, that binary will send the parameters over the named pipe for execution, and the results will be sent back

## Any bugs?
Yes:
- the command masqueraders can't correctly report the exact return code, it just returns 0 or 1
- it does not properly split out STDERR and STDOUT
- there have been situations where the temp dir isn't removed, and can cause collisions on subsequent executions
- it can't be executed in parallel with itself since the named pipe path is hardcoded
