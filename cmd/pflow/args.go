package main

import "flag"

// reorderArgs moves positional arguments after all flags so that flags may be
// written on either side of them.
//
// Go's flag package stops parsing at the first non-flag argument. Every command
// here takes its input file positionally and first, so the documented usage —
//
//	pflow simulate model.json --time 100
//
// left "--time 100" unparsed and silently used the default. Hoisting flags to
// the front before calling Parse makes both orderings work, which is what
// anyone typing these commands already expects.
//
// Flag values are kept with their flags: a non-boolean flag written as
// "-output file.svg" consumes the following argument, so it is not mistaken for
// a positional. A bare "--" terminates flag parsing; everything after it is
// treated as positional, matching the standard convention.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}

		if len(arg) < 2 || arg[0] != '-' {
			positional = append(positional, arg)
			continue
		}

		flags = append(flags, arg)

		// "-flag=value" carries its own value.
		name := flagName(arg)
		if name == "" || hasInlineValue(arg) {
			continue
		}

		// A non-boolean flag takes the next argument as its value.
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	if len(positional) == 0 {
		return flags
	}

	// Re-emit the terminator so Parse treats everything that follows as
	// positional. Without it, a positional that begins with "-" (a file named
	// "-weird.json", or anything after an explicit "--") would be parsed as a
	// flag and rejected.
	return append(append(flags, "--"), positional...)
}

// flagName strips leading dashes and any "=value" suffix.
func flagName(arg string) string {
	name := arg
	for len(name) > 0 && name[0] == '-' {
		name = name[1:]
	}
	for i := 0; i < len(name); i++ {
		if name[i] == '=' {
			return name[:i]
		}
	}
	return name
}

func hasInlineValue(arg string) bool {
	for i := 0; i < len(arg); i++ {
		if arg[i] == '=' {
			return true
		}
	}
	return false
}

// isBoolFlag reports whether a flag can be written without a value, matching
// the interface the flag package itself uses for this test.
func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}
