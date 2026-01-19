# MIXXLab

Go wrapper for [Mixxx](https://github.com/mixxxdj/mixxx) audio analysis (BPM and beat detection).

## Setup

```bash
git clone --recurse-submodules https://github.com/nzoschke/mixxxlab.git
cd mixxxlab
```

Or if already cloned:

```bash
git submodule update --init
```

## Build

Requires: cmake, libsndfile

```bash
# macOS
brew install cmake libsndfile

# Build the analyzer library
cd analyzer/lib
mkdir -p build && cd build
cmake .. && make
```

## Test

```bash
go test -v ./analyzer/...
```

Test fixtures from [The Wired CD (2004)](https://archive.org/details/The_WIRED_CD_Rip_Sample_Mash_Share-2769) (Creative Commons) are downloaded automatically on first run.
