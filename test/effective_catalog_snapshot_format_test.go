package test

import (
	"cmp"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

// snapshotIgnoredModelConfigFields lists display-only adaptor.ModelConfig fields
// that neither price a request nor bound it. They are excluded so prose edits do
// not drown real catalog changes in the snapshot diff.
var snapshotIgnoredModelConfigFields = map[string]bool{
	"Description":                 true,
	"HuggingFaceID":               true,
	"Quantization":                true,
	"SupportedFeatures":           true,
	"SupportedSamplingParameters": true,
}

// snapshotTimeType identifies time.Time so it renders as RFC 3339 instead of
// being walked as a struct.
var snapshotTimeType = reflect.TypeFor[time.Time]()

// formatSnapshotFloat renders a float with the shortest representation that
// round-trips, so the golden shows exactly the value billing uses.
//
// Parameters:
//   - v: the value to render.
//
// Returns: strconv.FormatFloat(v, 'g', -1, 64).
func formatSnapshotFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// snapshotSortedKeys returns the keys of m in ascending order.
//
// Parameters:
//   - m: any map with ordered keys.
//
// Returns: a new sorted slice of the keys.
func snapshotSortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	return slices.Sorted(maps.Keys(m))
}

// appendSnapshotValue appends the canonical key=value rendering of value to dst.
//
// Zero scalars, nil pointers and empty collections are omitted. A non-nil pointer
// whose fields are all zero renders as prefix={} because presence alone carries
// meaning for tariffs such as a free per-call price. Struct fields are named by
// their JSON tag, map entries are visited in sorted key order, and scalar slices
// render as [a,b,c].
//
// Parameters:
//   - dst: the field list to extend.
//   - prefix: the dotted path of value; empty for the top-level config.
//   - value: the value to render.
//   - ignored: Go struct field names to skip at every depth.
//
// Returns: dst extended with the rendered fields.
func appendSnapshotValue(dst []string, prefix string, value any, ignored map[string]bool) []string {
	return appendSnapshotReflect(dst, prefix, reflect.ValueOf(value), ignored)
}

// appendSnapshotReflect is the reflective worker behind appendSnapshotValue.
//
// Parameters:
//   - dst: the field list to extend.
//   - prefix: the dotted path of v.
//   - v: the reflected value to render.
//   - ignored: Go struct field names to skip.
//
// Returns: dst extended with the rendered fields.
func appendSnapshotReflect(dst []string, prefix string, v reflect.Value, ignored map[string]bool) []string {
	if !v.IsValid() {
		return dst
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return dst
		}
		before := len(dst)
		dst = appendSnapshotReflect(dst, prefix, v.Elem(), ignored)
		if len(dst) == before && v.Kind() == reflect.Pointer {
			dst = append(dst, prefix+"={}")
		}
		return dst
	case reflect.Struct:
		if v.Type() == snapshotTimeType {
			if t := v.Interface().(time.Time); !t.IsZero() {
				dst = append(dst, prefix+"="+t.UTC().Format(time.RFC3339))
			}
			return dst
		}
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if !field.IsExported() || ignored[field.Name] {
				continue
			}
			name := snapshotFieldName(field)
			if name == "" {
				continue
			}
			if prefix != "" {
				name = prefix + "." + name
			}
			dst = appendSnapshotReflect(dst, name, v.Field(i), ignored)
		}
		return dst
	case reflect.Map:
		if v.Len() == 0 {
			return dst
		}
		keys := v.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int {
			return strings.Compare(fmt.Sprint(a.Interface()), fmt.Sprint(b.Interface()))
		})
		for _, key := range keys {
			dst = appendSnapshotReflect(dst, prefix+"["+fmt.Sprint(key.Interface())+"]", v.MapIndex(key), ignored)
		}
		return dst
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return dst
		}
		if scalar, ok := snapshotScalarList(v); ok {
			return append(dst, prefix+"=["+scalar+"]")
		}
		for i := range v.Len() {
			dst = appendSnapshotReflect(dst, prefix+"["+strconv.Itoa(i)+"]", v.Index(i), ignored)
		}
		return dst
	default:
		if v.IsZero() {
			return dst
		}
		return append(dst, prefix+"="+snapshotScalar(v))
	}
}

// snapshotFieldName returns the JSON name of a struct field, falling back to
// the Go name when the field has no JSON tag.
//
// Parameters:
//   - field: the struct field.
//
// Returns: the name to render, or "" for fields tagged json:"-".
func snapshotFieldName(field reflect.StructField) string {
	tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	switch tag {
	case "-":
		return ""
	case "":
		return field.Name
	default:
		return tag
	}
}

// snapshotScalarList renders a slice of scalars as a comma-separated list.
//
// Parameters:
//   - v: a slice or array value.
//
// Returns: the joined rendering and true, or false when an element is not a scalar.
func snapshotScalarList(v reflect.Value) (string, bool) {
	parts := make([]string, 0, v.Len())
	for i := range v.Len() {
		element := v.Index(i)
		switch element.Kind() {
		case reflect.Pointer, reflect.Interface, reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
			return "", false
		}
		parts = append(parts, snapshotScalar(element))
	}
	return strings.Join(parts, ","), true
}

// snapshotScalar renders one scalar value canonically.
//
// Parameters:
//   - v: a bool, integer, float or string value.
//
// Returns: the rendering; strings are Go-quoted so they never contain spaces.
func snapshotScalar(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return formatSnapshotFloat(v.Float())
	case reflect.String:
		return strings.ReplaceAll(strconv.Quote(v.String()), " ", `\x20`)
	default:
		return fmt.Sprint(v.Interface())
	}
}

// parseSnapshotEntries splits snapshot text into entries keyed by
// "channelType<TAB>channelName<TAB>model", skipping comment lines.
//
// Parameters:
//   - text: the snapshot content.
//
// Returns: a map from entry key to its space-separated field list.
func parseSnapshotEntries(text string) map[string]string {
	entries := map[string]string{}
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\n")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			entries[line] = ""
			continue
		}
		entries[strings.Join(parts[:3], "\t")] = parts[3]
	}
	return entries
}

// diffSnapshotFields compares two space-separated field lists.
//
// Parameters:
//   - oldFields: the golden fields.
//   - newFields: the freshly generated fields.
//
// Returns: one "key: old -> new" description per changed field, sorted by key.
func diffSnapshotFields(oldFields, newFields string) []string {
	toMap := func(fields string) map[string]string {
		m := map[string]string{}
		for field := range strings.FieldsSeq(fields) {
			key, value, _ := strings.Cut(field, "=")
			m[key] = value
		}
		return m
	}
	oldMap, newMap := toMap(oldFields), toMap(newFields)
	keys := map[string]bool{}
	for key := range oldMap {
		keys[key] = true
	}
	for key := range newMap {
		keys[key] = true
	}
	var changes []string
	for _, key := range snapshotSortedKeys(keys) {
		oldValue, inOld := oldMap[key]
		newValue, inNew := newMap[key]
		switch {
		case !inOld:
			changes = append(changes, key+": (absent) -> "+newValue)
		case !inNew:
			changes = append(changes, key+": "+oldValue+" -> (absent)")
		case oldValue != newValue:
			changes = append(changes, key+": "+oldValue+" -> "+newValue)
		}
	}
	return changes
}

// describeEffectiveCatalogSnapshotDiff summarizes how got differs from want.
//
// Parameters:
//   - want: the committed golden.
//   - got: the freshly generated snapshot.
//   - limit: the maximum number of changed entries to describe.
//
// Returns: a human-readable report of added, removed and changed entries.
func describeEffectiveCatalogSnapshotDiff(want, got string, limit int) string {
	wantEntries, gotEntries := parseSnapshotEntries(want), parseSnapshotEntries(got)
	keys := map[string]bool{}
	for key := range wantEntries {
		keys[key] = true
	}
	for key := range gotEntries {
		keys[key] = true
	}

	var report []string
	added, removed, changed := 0, 0, 0
	for _, key := range snapshotSortedKeys(keys) {
		oldFields, inWant := wantEntries[key]
		newFields, inGot := gotEntries[key]
		label := strings.ReplaceAll(key, "\t", " / ")
		var line string
		switch {
		case !inWant:
			added++
			line = "+ " + label + ": " + newFields
		case !inGot:
			removed++
			line = "- " + label
		case oldFields != newFields:
			changed++
			line = "~ " + label + ": " + strings.Join(diffSnapshotFields(oldFields, newFields), "; ")
		default:
			continue
		}
		if len(report) < limit {
			report = append(report, line)
		}
	}
	if added+removed+changed == 0 {
		return "Entries are identical; only comments or ordering differ."
	}
	summary := fmt.Sprintf("%d added, %d removed, %d changed entries", added, removed, changed)
	if total := added + removed + changed; total > limit {
		summary += fmt.Sprintf(" (showing the first %d)", limit)
	}
	return summary + ":\n" + strings.Join(report, "\n")
}
