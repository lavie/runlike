package roundtrip

// Image area: the image the tag names changes or goes away between
// creating A and running runlike. runlike compares with the image the copy
// will run (the tag's), falls back to A's image ID, then to no image data,
// and warns each time.

// v1 and v2 set the same keys with other values, so every v1 value A
// holds must be printed for the copy (which runs v2) to match.
var (
	imgV1 = []string{`CMD ["echo","v1"]`, "ENV V=1", "WORKDIR /v1", "STOPSIGNAL SIGQUIT", "LABEL l=1", "HEALTHCHECK --interval=31s CMD true"}
	imgV2 = []string{`CMD ["echo","v2"]`, "ENV V=2", "WORKDIR /v2", "STOPSIGNAL SIGINT", "LABEL l=2", "HEALTHCHECK --interval=32s CMD false"}
)

func commit(tag string, changes ...string) []string {
	c := []string{"commit"}
	for _, ch := range changes {
		c = append(c, "--change", ch)
	}
	return append(c, "{p}-src", tag)
}

var (
	imgSrc      = []string{"create", "--name", "{p}-src", alpine}
	imgTeardown = [][]string{{"rm", "-f", "{p}-src"}, {"image", "rm", "-f", "{p}-img:t", "{p}-img:keep"}}
	movedImage  = []Ignore{{"Image", "the copy runs the image the tag names now, by design"},
		{"ImageManifestDescriptor.digest", "same: the new image's manifest"}}
)

var _ = add("image",
	Case{Name: "tag-moved",
		Setup:    [][]string{imgSrc, commit("{p}-img:t", imgV1...)},
		Args:     []string{"{p}-img:t"},
		AfterA:   [][]string{commit("{p}-img:t", imgV2...)},
		Teardown: append(imgTeardown, []string{"image", "rm", "-f", "{imageA}"}),
		Ignore:   movedImage,
		Warn:     []string{"the copy runs the new image"}},
	Case{Name: "tag-moved-adds",
		Setup:    [][]string{imgSrc, commit("{p}-img:t", "ENV V=1")},
		Args:     []string{"{p}-img:t"},
		AfterA:   [][]string{commit("{p}-img:t", "ENV V=1", "ENV NEW=x", "LABEL nl=y", "USER nobody")},
		Teardown: append(imgTeardown, []string{"image", "rm", "-f", "{imageA}"}),
		Ignore: append(movedImage,
			Ignore{"Config.Env", "the new image adds NEW=x; no flag removes it (warned)"},
			Ignore{"Config.Labels.nl", "the new image adds it; no flag removes it (warned)"},
			Ignore{"Config.User", "the new image sets USER; the original ran as the default user (warned)"}),
		Warn: []string{"it adds what no flag removes: env NEW, label nl, user nobody"}},
	// The old image is deleted after the tag moves (containerd store: a
	// rebuilt tag can take it away). runlike must not fail.
	Case{Name: "tag-moved-old-deleted",
		Setup:    [][]string{imgSrc, commit("{p}-img:t", imgV1...)},
		Args:     []string{"{p}-img:t"},
		AfterA:   [][]string{commit("{p}-img:t", imgV2...), {"image", "rm", "-f", "{imageA}"}},
		Teardown: imgTeardown,
		Ignore:   movedImage,
		Warn:     []string{"the copy runs the new image"}},
	// The tag is gone, A's image is not: compare with it. The tag comes
	// back before the copy, so the round trip is exact.
	Case{Name: "tag-gone",
		Setup:    [][]string{imgSrc, commit("{p}-img:t", imgV1...)},
		Args:     []string{"{p}-img:t"},
		AfterA:   [][]string{{"tag", "{p}-img:t", "{p}-img:keep"}, {"image", "rm", "{p}-img:t"}},
		BeforeB:  [][]string{{"tag", "{p}-img:keep", "{p}-img:t"}},
		Teardown: imgTeardown,
		Warn:     []string{"not found: compared with the container's image"}},
	// Tag and image gone: no image data, everything A holds is printed. A
	// rebuilt v1 (new ID) serves the copy.
	Case{Name: "image-gone",
		Setup:    [][]string{imgSrc, commit("{p}-img:t", imgV1...)},
		Args:     []string{"{p}-img:t"},
		AfterA:   [][]string{{"image", "rm", "-f", "{p}-img:t"}, {"image", "rm", "-f", "{imageA}"}},
		BeforeB:  [][]string{commit("{p}-img:t", "LABEL rebuilt=1")},
		Teardown: imgTeardown,
		Ignore:   append(movedImage, Ignore{"Config.Labels.rebuilt", "marks the rebuilt image only"}),
		Warn:     []string{"nor the container's image", "no image data"}},
)
