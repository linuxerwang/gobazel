# gobazel

gobazel is a tool to help golang bazel developers to map bazel's folder
structure to golang's standard folder structure, through FUSE & FsNotify
(thus only works for Linux users).

## Why gobazel

First, I love to use [bazel](https://bazel.io) and
[rules_go](https://github.com/bazelbuild/rules_go) to manage my projects.
Bazel is created by Google for huge amounts of inter-related projects
(imagine you have thousands of projects and they depend on each other in
the most complex way). Besides the many great features of Bazel, its way
of dependency management makes it much easier to manage such projects.

### What's the Problem with bazel (and rules_go)?

Bazel's folder layout is very different from most existing languages,
thus causes many problems for developers. The top issue is the poor IDE
and tools support. Bazel team released demo plugins for Eclipse and Intellij,
but none of them works smoothly for me.

For Golang programming, the problem is much worse. The standard Golang project
structure is with a GOPATH, in which three standard folders are expected: bin,
pkg, and src. All source codes are put in src folder, both your code and the
dependant 3rd-party code. The following shows a typical GOPATH layout:

```
$TOP
  |- bin/
  |- pkg/
  |- src/
      |- github.com/
      |   |- ...
      |
      |- mycompany.com/
          |- my-prod-1/
          |- my-prod-2/
          |- my-prod-3/
          |- ...
```

In comparison, the same projects can be organized better with bazel:

```
$TOP
  |- bazel-bin/
  |- bazel-genfiles/
  |- ...
  |- my-prod-1/
  |- my-prod-2/
  |- my-prod-3/
  |- ...
  |- third-party-go/
  |   |- vendor/
  |       |- github.com/
  |           |- ...
  |- WORKSPACE
```

rules_go lets you specify a Go package prefix, for example "mycompany.com",
which is not represented in the above layout. Instead, rules_go uses certain
techniques to trick the Go compiler to work the prefix around.

It causes serious problems because there is no GOPATH any more, and most of
the existing Golang tools depend on the GOPATH! So now godoc stops working,
go-guru stops working, and most IDEs stop working.

### gobazel to the Rescue

So Go tools and IDEs requires a GOPATH, which is not in alignment with bazel's
way of organizing source codes. That's why gobazel was born: utilizing the
FUSE virtual file system, it simulates a $GOPATH/src by mapping folders
properly, thus satisfies both sides:

- A top folder outside of bazel workspace should be created as the GOPATH.

- Within $GOPATH three folders should be created: bin, src, pkg.

- FUSE mount the virtual file system on $GOPATH/src.

- A $GOPATH/src/<go-pkg-prefix> is simulated, such as:
	$GOPATH/src/mycompany.com.

- All top level folders (my-prod-1, ...) except the third-party-go will
	be mapped under $GOPATH/src/<go-pkg-prefix>.

- All folders under third-party-go/vendor will be mapped to under $GOPATH/src.

- The bazel-* links will be ignored, except that all entries in bazel-genfiles
	will be mapped under $GOPATH/src/<go-pkg-prefix>.

Once the above has been done, the GOPATH can be set to the new top folder and
everything else will just work by itself, because from Golang tools it's
simply a real GOPATH.

## Installation and Setup

```bash
$ go get github.com/linuxerwang/gobazel
```

Make sure the compiled command "gobazel" be put into your $PATH. Yes, you
need a normal GOPATH to go get gobazel. Once you get the command, you don't
need this GOPATH any more.

Next, make a new empty GOPATH. Suppose your bazel workspace is at ~/my-bazel
and your Go package prefix is "mycompany.com". You could create it at
~/my-bazel-gopath.

Execute command "gobazel" under ~/my-bazel:

```bash
me@laptop:~/my-bazel$ gobazel
Created gobazel config file /home/me/my-bazel/.gobazelrc,
please customize it and run the command again.
```

Customize the file .gobazelrc to fit your layout. Mine looks like:

```
gobazel {
    go-path: "/home/me/my-bazel-gopath"
    go-pkg-prefix: "mycompany.com"
    go-ide-cmd: "/usr/bin/atom"
    # go-ide-cmd: "/home/tools/liteide/bin/liteide"

    vendor-dirs: [
        "third-party-go/vendor",
    ]

    ignore-dirs: [
        "bazel-.*",
        "third-party.*",
    ]

    fall-through-dirs: [
        ".vscode",
    ]
}
```

You can set up your favorite IDE, or specify empty.

The last step, execute "gobazel" command again (in the bazel workspace),
and you should see the IDE launched and everything worked.

Flag --build enables gobazel to build all bazel targets which satisfy the
given criteria. An example config looks as follows:

```
gobazel {
    ...

    build {
        rules: [
            "genproto_go",
        ]
        ignore-dirs: [
            "bazel-.*",
            "third-party.*",
        ]
    }

    ...
}

```

Flag --debug enables gobazel to print out verbose debug information.

## Remove Debug with Delve (dlv)

Start your binary with dlv:

```bash
$ dlv exec bazel-bin/myserver/cmd/myserver/myserver --headless --listen=:2345 --log [-- <other-args>]
```

In VS code, add a debug configuration:

```js
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "bazel-bin/myserver/cmd/myserver/myserver",
            "type": "go",
            "request": "launch",
            "mode": "remote",
            "remotePath": "/home/linuxerwang/.cache/bazel/_bazel_linuxerwang/eedeac95b950221f7e2a454b8c435113/bazel-sandbox/93167466216111235/execroot/__main__",
            "port": 2345,
            "host": "127.0.0.1",
            "program": "${workspaceRoot}/mycompany.com",
            "env": {},
            "args": []
        }
    ]
}
```

The "remotePath" field can be extracted with gdb:

```
$ gdb bazel-bin/myserver/cmd/myserver/myserver -batch -ex "list" -ex "info source"
warning: Missing auto-load script at offset 0 in section .debug_gdb_scripts of file /home/linuxerwang/.cache/bazel/_bazel_linuxerwang/eedeac95b950221f7e2a454b8c435113/execroot/__main__/bazel-out/k8-dbg/bin/myserver/cmd/myserver/myserver.
Use `info auto-load python-scripts [REGEXP]' to list them.
warning: Source file is more recent than executable.
24              flagFile        = flag.String("f", "", "The CSV file")
25
26              success, failure int32
27      )
28
29      func usage() {
30              fmt.Println("Usage:")
Current source file is myserver/cmd/myserver/main.go
Compilation directory is /home/linuxerwang/.cache/bazel/_bazel_linuxerwang/eedeac95b950221f7e2a454b8c435113/bazel-sandbox/93167466216111235/execroot/__main__
Located in /home/linuxerwang/my-client/myserver/cmd/myserver/main.go
Contains 129 lines.
Source language is asm.
Producer is Go cmd/compile go1.9.2.
Compiled with DWARF 2 debugging format.
Does not include preprocessor macro info.
```

Note that the path is in the line starting with "Compilation directory".

Now you can do remote debug in vscode with F5. It can trace the Go-SDK (using
the Go-SDK in bazel external directory) and third party code correctly.

## Caveates

- At present it only works on Linux and OSX (thanks excavador for adding the OSX
    support). It requires FUSE, fsnotify.

- It works for Golang programming with bazel. Might also work for Java
	after code change (not planned for now).

- Do not change files both in bazel workspace and in the simulated GOPATH!
	At right now the fsnotify doesn't work between the simulated GOPATH
	and bazel workspace. So if a file is changed in bazel workspace,
	the IDE (on simulated GOPATH) will not notice the change thus you might
	overwrite files accidentally (and even without realizing it). Some
	reference documents:

	- https://github.com/libfuse/libfuse/wiki/Fsnotify-and-FUSE
	- https://www.howtoforge.com/tutorial/monitoring-file-changes-with-linux-over-the-network/

	I didn't make it work. Fix's welcome.

- For files generated in bazel-genfiles, you have to manually run bazel
	command in bazel workspace. gobazel will not automatically run it.

- Tested on LiteIDE and Atom. Tested godoc, go-guru.

- The file deletion in Atom is through moving to trash, which is not
	supported in gobazel. You can install the package "permanent-delete":
	https://atom.io/packages/permanent-delete.

## Acknowledgement

- FUSE bindings for Go: https://github.com/hanwen/go-fuse
- File system event notification: https://github.com/rjeczalik/notify
