package components

import "github.com/teapotovh/teapot/lib/ui"

var WireframeStyle = ui.MustParseStyle(`
	padding-left: var(--size-1);
	padding-right: var(--size-1);

	@media (min-width: 1024px) {
	  padding-left: var(--size-2);
	  padding-right: var(--size-2);
	}

	@media (min-width: 1440px) {
	  padding-left: var(--size-4);
	  padding-right: var(--size-4);
		border-left: 1px dashed var(--theme-wireframe-0);
		border-right: 1px dashed var(--theme-wireframe-0);
	}
`)
