# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

huesync — a Go application that syncs screen colors to Philips Hue lights. Module name: `huesync`.

## Overview

The program continuously captures the screen at configurable intervals, calculates the average color value of the entire screen, and sends that color to lamps in a Philips Hue entertainment area via a Hue bridge.

## Build & Run

- **Build:** `go build ./...`
- **Run:** `go run .`
- **Test all:** `go test ./...`
- **Test single:** `go test ./path/to/package -run TestName`
- **Vet:** `go vet ./...`

## Git

Do not include any mention of Claude or co-author lines in commit messages.
