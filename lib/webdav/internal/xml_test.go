package internal

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

func TestNewRawXMLElementMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value *RawXMLValue
		want  string
	}{
		{
			name:  "known namespace",
			value: NewRawXMLElement(ResourceTypeName, nil, nil),
			want:  `<resourcetype xmlns="DAV:"></resourcetype>`,
		},
		{
			name:  "unknown namespace",
			value: NewRawXMLElement(xml.Name{Space: "http://example.com/ns", Local: "custom"}, nil, nil),
			want:  `<custom xmlns="http://example.com/ns"></custom>`,
		},
		{
			name: "attribute",
			value: NewRawXMLElement(DisplayNameName, []xml.Attr{
				{Name: xml.Name{Local: "lang"}, Value: "en"},
			}, nil),
			want: `<displayname xmlns="DAV:" lang="en"></displayname>`,
		},
		{
			name: "text content",
			value: NewRawXMLElement(DisplayNameName, nil, []RawXMLValue{
				{tok: xml.CharData(testDisplayName)},
			}),
			want: `<displayname xmlns="DAV:">home</displayname>`,
		},
		{
			name: "nested elements",
			value: NewRawXMLElement(ResourceTypeName, nil, []RawXMLValue{
				*NewRawXMLElement(CollectionName, nil, nil),
			}),
			want: `<resourcetype xmlns="DAV:"><collection xmlns="DAV:"></collection></resourcetype>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := xml.Marshal(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestEncodeRawXMLElement(t *testing.T) {
	t.Parallel()

	val, err := EncodeRawXMLElement(&DisplayName{Name: testDisplayName})
	require.NoError(t, err)

	name, ok := val.XMLName()
	assert.False(t, ok)
	assert.Equal(t, xml.Name{}, name)

	got, err := xml.Marshal(val)
	require.NoError(t, err)
	assert.Equal(t, `<displayname xmlns="DAV:">home</displayname>`, string(got))
}

func TestRawXMLValueRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "nested elements",
			input: `<resourcetype xmlns="DAV:"><collection xmlns="DAV:"></collection></resourcetype>`,
		},
		{
			name:  "namespace and attribute",
			input: `<displayname xmlns="DAV:" xml:lang="en">home</displayname>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var val RawXMLValue

			require.NoError(t, xml.Unmarshal([]byte(tt.input), &val))

			got, err := xml.Marshal(&val)
			require.NoError(t, err)
			assert.Equal(t, tt.input, string(got))
		})
	}
}

func TestRawXMLValueDecodeResourceType(t *testing.T) {
	t.Parallel()

	var val RawXMLValue

	require.NoError(
		t,
		xml.Unmarshal([]byte(`<resourcetype xmlns="DAV:"><collection></collection></resourcetype>`), &val),
	)

	name, ok := val.XMLName()
	require.True(t, ok)
	assert.Equal(t, ResourceTypeName, name)

	var rt ResourceType

	require.NoError(t, val.Decode(&rt))
	assert.True(t, rt.Is(CollectionName))
}

func TestRawXMLValueDecode(t *testing.T) {
	t.Parallel()

	var val RawXMLValue

	require.NoError(t, xml.Unmarshal([]byte(`<displayname xmlns="DAV:">home</displayname>`), &val))

	var name DisplayName

	require.NoError(t, val.Decode(&name))
	assert.Equal(t, testDisplayName, name.Name)
	assert.Equal(t, DisplayNameName, name.XMLName)
}

func TestRawXMLValueUnmarshalInvalidXML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "truncated start element", input: `<resourcetype`},
		{name: "unclosed element", input: `<resourcetype></resourcetype`},
		{name: "mismatched end element", input: `<resourcetype><collection></resourcetype>`},
		{name: "stray end element", input: `</resourcetype>`},
		{name: "plain text", input: `not xml`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var val RawXMLValue
			assert.Error(t, xml.Unmarshal([]byte(tt.input), &val))
		})
	}
}

type missingXMLNameField struct {
	Value string
}

type wrongXMLNameType struct {
	XMLName string
}

type missingXMLTag struct {
	XMLName xml.Name
}

type unqualifiedXMLTag struct {
	XMLName xml.Name `xml:"displayname"`
}

func TestValueXMLNameErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		wantErr error
	}{
		{name: "not a struct", value: testDisplayName, wantErr: ErrNotStruct},
		{name: "pointer to not a struct", value: new(int), wantErr: ErrNotStruct},
		{name: "missing XMLName field", value: missingXMLNameField{}, wantErr: ErrMissingXMLNameStructField},
		{name: "XMLName is not xml.Name", value: wrongXMLNameType{}, wantErr: ErrNotXMLName},
		{name: "missing xml tag", value: missingXMLTag{}, wantErr: ErrMissingXMLTag},
		{name: "unqualified xml tag", value: unqualifiedXMLTag{}, wantErr: ErrExpectedNamespaceLocalName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := (&Prop{}).Decode(tt.value)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestPropDecodeKnownNameMissingValue(t *testing.T) {
	t.Parallel()

	err := (&Prop{}).Decode(&DisplayName{Name: testDisplayName})
	require.Error(t, err)
	assert.True(t, daverr.IsNotFound(err))
	assert.True(t, strings.Contains(err.Error(), "missing property"))
}
