## Go Templates

This directory contains a series of go scripts (`package main`) with template variables like `{{ .Foo }}` added. They are intended to be run _from tests_ in order to execute a small part of the test in a different process, usually for testing SIGKILL behaviour.

As these scripts are not actually tests, helper functions which accept a `t` should use `api.MockT` instead.