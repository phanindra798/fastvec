"""Measure standalone Go loader test binaries on Windows, in alternating order.

Example: python py/measure_load_windows.py --before before.exe --after after.exe
         --index D:/data/sift.fvi --out bench/results/load-comparison.json
"""
import argparse
import ctypes
from ctypes import wintypes
import json
import os
from pathlib import Path
import re
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--before', required=True, type=Path)
    parser.add_argument('--after', required=True, type=Path)
    parser.add_argument('--index', required=True, type=Path)
    parser.add_argument('--out', required=True, type=Path)
    args = parser.parse_args()
    if os.name != 'nt': parser.error('Windows process memory APIs are required')
    class Counters(ctypes.Structure):
        _fields_ = [('cb', wintypes.DWORD), ('faults', wintypes.DWORD)] + [
            (name, ctypes.c_size_t) for name in ('peak_working_set', 'working_set',
            'peak_paged', 'paged', 'peak_nonpaged', 'nonpaged', 'pagefile', 'peak_pagefile')]
    kernel = ctypes.WinDLL('kernel32', use_last_error=True)
    psapi = ctypes.WinDLL('psapi', use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    psapi.GetProcessMemoryInfo.argtypes = [wintypes.HANDLE, ctypes.POINTER(Counters), wintypes.DWORD]
    env = dict(os.environ, FASTVEC_LOAD_MEASURE=str(args.index.resolve()), GOMAXPROCS='1')
    results = []
    for pair in range(3):
        for variant in (('before', 'after') if pair != 1 else ('after', 'before')):
            command = [str(getattr(args, variant).resolve()), '-test.run=^TestLoadMeasurement$', '-test.v']
            with subprocess.Popen(command, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True) as process:
                handle = kernel.OpenProcess(0x410, False, process.pid)
                if not handle:
                    process.terminate()
                    raise ctypes.WinError(ctypes.get_last_error())
                peak = 0
                try:
                    while process.poll() is None:
                        counters = Counters()
                        counters.cb = ctypes.sizeof(counters)
                        if psapi.GetProcessMemoryInfo(handle, ctypes.byref(counters), counters.cb):
                            peak = max(peak, counters.peak_working_set)
                        time.sleep(.01)
                finally:
                    kernel.CloseHandle(handle)
                output = process.stdout.read()
                if process.returncode or not peak: raise RuntimeError(output or 'no memory samples')
            match = re.search(r'load_ns=(\d+) allocated_bytes=(\d+) allocations=(\d+) heap_bytes=(\d+) nodes=(\d+)', output)
            if not match: raise RuntimeError(output)
            record = dict(zip(('load_ns','allocated_bytes','allocations','heap_bytes','nodes'), map(int,match.groups())))
            record.update(pair=pair+1, variant=variant, peak_working_set_bytes=peak)
            results.append(record)
            print(record, flush=True)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    with args.out.open('x', encoding='utf-8') as f:
        json.dump(results, f, indent=2)
        f.write('\n')


if __name__ == '__main__':
    main()
