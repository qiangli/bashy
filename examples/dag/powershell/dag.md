---
name: powershell
description: bashy dag front door for PowerShell — exercises Bash#-owned control flow calling fenced PowerShell functions
type: dag
default: smoke
---

# PowerShell — task graph

Demonstrates Bash#-owned control flow calling fenced PowerShell functions via
the pinned PowerShell 7 runtime (`pwsh`).

## Tasks

### test
Run a focused test verifying PowerShell function call and pipeline filter.
Effects: exec

```bsh
~~~powershell as ps
function AddNumbers {
    [OutputType([long])]
    param([long]$a, [long]$b)
    return ($a + $b)
}
~~~
res := ps.AddNumbers(15, 27)
[ "$res" -eq 42 ] || exit 1
echo "test: powershell ok (res=$res)"
```

### smoke
Exercise Bash#-owned control flow calling fenced PowerShell functions:
loops, conditionals, error handling, and pipeline command forms.
Effects: read, exec

```bsh
~~~powershell as ps
function Square {
    [OutputType([long])]
    param([long]$n)
    return ($n * $n)
}

function Shout {
    process { $_.ToUpperInvariant() }
}

function ValidatePositive {
    param([int]$n)
    if ($n -lt 0) {
        throw "value must be positive: $n"
    }
    return "ok:$n"
}
~~~

# 1. Bash#-owned loop accumulating results from fenced PowerShell
sum=0
for n in 1 2 3 4; do
    sq := ps.Square(n)
    sum=$((sum + sq))
done
echo "smoke: sum_squares=$sum"
[ "$sum" -eq 30 ] || exit 1

# 2. Bash#-owned conditional branching on PowerShell output
check := ps.Square(7)
if [ "$check" -gt 40 ]; then
    echo "smoke: branch=gt40"
else
    echo "smoke: branch=le40" >&2
    exit 1
fi

# 3. Pipeline command form through PowerShell process filter
shouted=$(printf 'gamma\nalpha\nbeta\n' | ps.Shout | sort)
[ "$shouted" = $'ALPHA\nBETA\nGAMMA' ] || exit 1
echo "smoke: shout_pipeline=ok"

# 4. Error binding and status handling
neg=-5
val, err := ps.ValidatePositive(neg)
[ -z "$val" ] || exit 1
[[ "$err" == *positive* ]] || exit 1
echo "smoke: error_handling=ok"

echo "smoke: powershell PASS"
```
