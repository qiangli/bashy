---
name: csharp
description: bashy dag front door for C# — exercises Bash#-owned control flow calling fenced C# methods
type: dag
default: smoke
---

# C# — task graph

Demonstrates Bash#-owned control flow calling fenced C# declaration units
compiled via Add-Type on the pinned PowerShell 7 runtime (`pwsh`).

## Tasks

### test
Run a focused test verifying C# method execution and result mapping.
Effects: exec

```bsh
~~~csharp as cs
using System;

public static long Multiply(long a, long b) => a * b;
public static string Greet(string name) => $"hello, {name}";
~~~
res := cs.Multiply(6, 7)
[ "$res" -eq 42 ] || exit 1
msg := cs.Greet("bashy")
[ "$msg" = "hello, bashy" ] || exit 1
echo "test: csharp ok (res=$res, msg=$msg)"
```

### smoke
Exercise Bash#-owned control flow calling fenced C# static methods:
loops, conditionals, string joins, and exception error handling.
Effects: read, exec

```bsh
~~~csharp as cs
using System;
using System.Linq;

public static long Factorial(long n) {
    long r = 1;
    for (long i = 2; i <= n; i++) r *= i;
    return r;
}

public static string FormatTag(string prefix, long id) => $"{prefix}:{id}";

public static string JoinItems(string sep, long count) =>
    string.Join(sep, Enumerable.Range(1, (int)count));

public static string ValidateInput(string s) {
    if (string.IsNullOrEmpty(s)) {
        throw new ArgumentException("input cannot be empty");
    }
    return $"valid:{s}";
}
~~~

# 1. Bash#-owned loop accumulating factorial calculations from C#
sum=0
for n in 1 2 3 4; do
    f := cs.Factorial(n)
    sum=$((sum + f))
done
echo "smoke: sum_factorial=$sum"
[ "$sum" -eq 33 ] || exit 1

# 2. Bash#-owned conditional branching on C# results
tag := cs.FormatTag("worker", 42)
if [ "$tag" = "worker:42" ]; then
    echo "smoke: tag_match=ok"
else
    echo "smoke: tag_mismatch" >&2
    exit 1
fi

# 3. String joining across boundary
joined := cs.JoinItems("-", 3)
echo "smoke: join=$joined"
[ "$joined" = "1-2-3" ] || exit 1

# 4. Exception error handling and error-variable binding
ok_val, ok_err := cs.ValidateInput("payload")
[ "$ok_val" = "valid:payload" ] || exit 1
[ -z "$ok_err" ] || exit 1

bad_val, bad_err := cs.ValidateInput("")
[ -z "$bad_val" ] || exit 1
[[ "$bad_err" == *ArgumentException*empty* ]] || exit 1
echo "smoke: error_handling=ok"

echo "smoke: csharp PASS"
```
