Improved GOTK3 (go bindings for GTK+3)
======================================

[![Build Status](https://travis-ci.org/d2r2/gotk3.svg?branch=master)](https://travis-ci.org/d2r2/gotk3)
[![Go Report Card](https://goreportcard.com/badge/github.com/d2r2/gotk3)](https://goreportcard.com/report/github.com/d2r2/gotk3)
[![GoDoc](https://godoc.org/github.com/d2r2/gotk3?status.svg)](https://godoc.org/github.com/d2r2/gotk3)
<!--
[![Coverage Status](https://coveralls.io/repos/d2r2/gotk3/badge.svg?branch=master)](https://coveralls.io/r/d2r2/gotk3?branch=master)
-->


Original [GOTK3](https://godoc.org/github.com/gotk3) project provides Go bindings for GTK+3 and dependent
projects. Partial binding support for the
following libraries is currently implemented:

  - GTK+3 (3.12 and later)
  - GDK 3 (3.12 and later)
  - GLib 2 (2.36 and later)
  - Cairo (1.10 and later)

Idea to create a fork from the original one of the best
[GTK+3 adaptation for golang](https://godoc.org/github.com/gotk3) was
not just add another API functions, but came from the other side - create
good application example written in go, which demostrate modern GTK+ features
(as well as old-style one) and in addition provide out-of-the-box code patterns.

As a results a lot of refactoring was done with original code, to create on
the top example application CoolApp, which contains many ready to use code samples
to build go-application with modern GTK+3 GUI.

Short list of changes made:
1) Code refactored and reformated for better mix of widgets in corresponding files.
2) Some amount of error was fixed including memory leaks.
3) GOTK3 examples which in original version located separately, here integrated in one project.
4) New GLIB, GTK+ objects and widgets supported, including GAction, GSimpleAction, GActionMap,
GMenuModel, GMenu, GtkActionable, GFile and so on.
5) CoolApp example application created to demonstrate golang code patterns to build
modern GTK+3 application, which incude menus, toolbars, actions and others
widgets and tools (including pattern for fullscreen wrap/unwrap, preference dialog,
save/restore settings and so on).

## Documentation

Each package's internal `go doc` style documentation can be viewed
online without installing this package by using the GoDoc site (links
to [cairo](http://godoc.org/github.com/d2r2/gotk3/cairo),
[glib](http://godoc.org/github.com/d2r2/gotk3/glib),
[gdk](http://godoc.org/github.com/d2r2/gotk3/gdk), and
[gtk](http://godoc.org/github.com/d2r2/gotk3/gtk) documentation).

You can also view the documentation locally once the package is
installed with the `godoc` tool by running `godoc -http=":6060"` and
pointing your browser to
http://localhost:6060/pkg/github.com/d2r2/gotk3

## Installation

gotk3 currently requires GTK 3.6-3.16, GLib 2.36-2.40, and
Cairo 1.10 or 1.12.  A recent Go (1.6 or newer) is also required.

For detailed instructions see the wiki pages: [installation](https://github.com/d2r2/gotk3/wiki#installation)

## License

Modified GOTK3 is licensed under the liberal ISC License, as the original version.
