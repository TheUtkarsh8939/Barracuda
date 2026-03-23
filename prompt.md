

You are refactoring a Go chess engine to replace its current chess library dependency with a custom high-performance bitboard library.

### Goal

Replace **all usage of**

```
github.com/corentinGS/chess/v2
```

with the custom library:

```
github.com/TheUtkarsh8939/bitboardChess
```

which is already present locally in the project directory:

```
./bitboardChess
```

---

# Important Constraint

The following library **must NOT be modified or replaced** because it is a separate dependency used for opening books:

```
github.com/corentings/v2/opening
```

Any imports or code referencing that package must remain unchanged.

---

# Refactoring Requirements

## 1. Replace All Chess Engine Logic

Remove usage of:

```
github.com/corentinGS/chess/v2
```

and rewrite the implementation so that it exclusively uses:

```
github.com/TheUtkarsh8939/bitboardChess
```

The new implementation must rely on the **bitboard representation and APIs provided by that library**.

---

# 2. Performance Requirements

The refactored code must be **as performant as possible**.

Prefer:

### Bitwise operations

```
&
|
^
<<
>>
```

### Lookup tables

Use precomputed tables whenever possible.

### Bitboard iteration

Use patterns like:

```go
sq := bits.TrailingZeros64(bb)
bb &= bb - 1
```

Avoid:

* expensive object allocations
* reflection
* unnecessary conversions
* nested loops when bitwise masks can be used

Minimize use of:

```
for range
```

when bitboard iteration is possible.

---

# 3. Assistive Functions

Any helper or utility functions required for the refactor must **NOT be placed in existing files**.

Instead:

Create a new file:

```
library_extension.go
```

All newly written helper functions must be placed there.

Examples include:

* board conversion utilities
* bitboard helpers
* move translation utilities
* lookup helpers
* attack detection helpers

Do not scatter helper functions across the project.

---

# 4. Code Organization

Ensure the refactored code:

* compiles successfully
* uses idiomatic Go
* minimizes allocations
* keeps the engine architecture intact
* avoids breaking public interfaces if possible

---

# 5. Migration Strategy

Perform the migration safely by:

1. locating every import of

```
github.com/corentinGS/chess/v2
```

2. rewriting logic to use `bitboardChess`

3. adapting data structures where necessary

4. implementing compatibility helpers in:

```
library_extension.go
```

5. removing unused imports

---

# 6. Bitboard Optimization Expectations

Wherever applicable:

Prefer patterns like:

```go
bb := pieces
for bb != 0 {
    sq := bits.TrailingZeros64(bb)
    bb &= bb - 1
}
```

Instead of iterating through squares 0–63.

Use **bit masks and attack tables** where appropriate.

---

# 7. Do Not Break Opening Book Integration

Any usage of:

```
github.com/corentings/v2/opening
```

must remain untouched.

Do not refactor or replace code related to this dependency.

---

# 8. Output Expectations

Perform a **full refactor of the codebase** that:

* removes the dependency on `corentinGS/chess/v2`
* integrates `bitboardChess`
* adds a new file `library_extension.go` containing all helper utilities
* maintains functionality
* improves performance through bitboards and lookup tables

---

⚙️ **Optional but recommended behavior**

If any functionality from `corentinGS/chess/v2` is missing in `bitboardChess`, implement the required logic using **bitboard operations in `library_extension.go`**, prioritizing speed and low allocations.

---