package copy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/internal/private"
	internalsignature "go.podman.io/image/v5/internal/signature"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/types"
)

// mockImageSource is a simple mock for types.ImageSource
type mockImageSource struct {
	t *testing.T
	configBlob []byte
	layerBlob  []byte
	manifest   []byte
	ref        types.ImageReference
}

func (m *mockImageSource) Reference() types.ImageReference { return m.ref }
func (m *mockImageSource) Close() error                    { return nil }
func (m *mockImageSource) GetManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, string, error) {
	return m.manifest, manifest.DockerV2Schema2MediaType, nil
}
func (m *mockImageSource) GetBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache) (io.ReadCloser, int64, error) {
	if info.Digest == digest.FromBytes(m.configBlob) {
		return io.NopCloser(bytes.NewReader(m.configBlob)), int64(len(m.configBlob)), nil
	}
	if info.Digest == digest.FromBytes(m.layerBlob) {
		return io.NopCloser(bytes.NewReader(m.layerBlob)), int64(len(m.layerBlob)), nil
	}
	return nil, 0, errors.New("blob not found")
}
func (m *mockImageSource) HasThreadSafeGetBlob() bool { return true }
func (m *mockImageSource) SupportsGetBlobAt() bool    { return false }
func (m *mockImageSource) GetBlobAt(ctx context.Context, info types.BlobInfo, offset, length int64) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (m *mockImageSource) GetSignaturesWithFormat(ctx context.Context, instanceDigest *digest.Digest) ([]internalsignature.Signature, error) {
	return nil, nil
}
func (m *mockImageSource) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	return nil, nil
}
func (m *mockImageSource) LayerInfosForCopy(ctx context.Context, instanceDigest *digest.Digest) ([]types.BlobInfo, error) {
	return nil, nil
}

// mockImageDestination is a simple mock for private.ImageDestination
type mockImageDestination struct {
	t *testing.T
	configUploaded chan struct{}
	layerUploadBlock chan struct{}
	ref types.ImageReference

	mu sync.Mutex
	configUploadedClosed bool
}

func (m *mockImageDestination) Reference() types.ImageReference { return m.ref }
func (m *mockImageDestination) Close() error                    { return nil }
func (m *mockImageDestination) SupportedManifestMIMETypes() []string {
	return []string{manifest.DockerV2Schema2MediaType}
}
func (m *mockImageDestination) SupportsSignatures(ctx context.Context) error { return nil }
func (m *mockImageDestination) DesiredLayerCompression() types.LayerCompression {
	return types.PreserveOriginal
}
func (m *mockImageDestination) AcceptsForeignLayerURLs() bool { return false }
func (m *mockImageDestination) MustMatchRuntimeOS() bool      { return false }
func (m *mockImageDestination) IgnoresEmbeddedDockerReference() bool { return false }
func (m *mockImageDestination) HasThreadSafePutBlob() bool    { return true }
func (m *mockImageDestination) SupportsPutBlobPartial() bool  { return false }

func (m *mockImageDestination) PutBlobWithOptions(ctx context.Context, stream io.Reader, inputInfo types.BlobInfo, options private.PutBlobOptions) (private.UploadedBlob, error) {
	if options.IsConfig {
		m.mu.Lock()
		if !m.configUploadedClosed {
			close(m.configUploaded)
			m.configUploadedClosed = true
		}
		m.mu.Unlock()
		return private.UploadedBlob{Digest: inputInfo.Digest, Size: inputInfo.Size}, nil
	}
	// Layer
	<-m.layerUploadBlock
	return private.UploadedBlob{Digest: inputInfo.Digest, Size: inputInfo.Size}, nil
}

func (m *mockImageDestination) PutManifest(ctx context.Context, man []byte, instanceDigest *digest.Digest) error {
	return nil
}
func (m *mockImageDestination) PutSignaturesWithFormat(ctx context.Context, signatures []internalsignature.Signature, instanceDigest *digest.Digest) error {
	return nil
}
func (m *mockImageDestination) TryReusingBlobWithOptions(ctx context.Context, info types.BlobInfo, options private.TryReusingBlobOptions) (bool, private.ReusedBlob, error) {
	return false, private.ReusedBlob{}, nil
}
func (m *mockImageDestination) TryReusingBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache, substitute bool) (bool, types.BlobInfo, error) {
	return false, types.BlobInfo{}, nil
}
func (m *mockImageDestination) CommitWithOptions(ctx context.Context, options private.CommitOptions) error {
	return nil
}
func (m *mockImageDestination) Commit(ctx context.Context, unparsedToplevel types.UnparsedImage) error {
	return nil
}
func (m *mockImageDestination) PutSignatures(ctx context.Context, signatures [][]byte, instanceDigest *digest.Digest) error {
	return nil
}

func (m *mockImageDestination) PutBlobPartial(ctx context.Context, chunkAccessor private.BlobChunkAccessor, srcInfo types.BlobInfo, options private.PutBlobPartialOptions) (private.UploadedBlob, error) {
	return private.UploadedBlob{}, errors.New("not implemented")
}
func (m *mockImageDestination) NoteOriginalOCIConfig(ociConfig *imgspecv1.Image, configErr error) error {
    return nil
}

// mockImageTransport
type mockImageTransport struct {}
func (t *mockImageTransport) Name() string { return "mock" }
func (t *mockImageTransport) ParseReference(reference string) (types.ImageReference, error) { return nil, nil }
func (t *mockImageTransport) ValidatePolicyConfigurationScope(scope string) error { return nil }

// mockImageReference implementation
type mockImageReference struct {
	src *mockImageSource
	dest *mockImageDestination
}
func (r *mockImageReference) Transport() types.ImageTransport { return &mockImageTransport{} }
func (r *mockImageReference) StringWithinTransport() string   { return "mock" }
func (r *mockImageReference) DockerReference() reference.Named { return nil }
func (r *mockImageReference) String() string { return "mock:image" }
func (r *mockImageReference) PolicyConfigurationIdentity() string { return "mock:image" }
func (r *mockImageReference) PolicyConfigurationNamespaces() []string { return nil }
func (r *mockImageReference) NewImageSource(ctx context.Context, sys *types.SystemContext) (types.ImageSource, error) {
	return r.src, nil
}
func (r *mockImageReference) NewImageDestination(ctx context.Context, sys *types.SystemContext) (types.ImageDestination, error) {
	return r.dest, nil // Returns our private mock as public interface
}
func (r *mockImageReference) NewImage(ctx context.Context, sys *types.SystemContext) (types.ImageCloser, error) {
	return nil, errors.New("not implemented")
}
func (r *mockImageReference) DeleteImage(ctx context.Context, sys *types.SystemContext) error { return nil }

func (r *mockImageReference) NewImageDestinationPrivate(ctx context.Context, sys *types.SystemContext) (private.ImageDestination, error) {
	return r.dest, nil
}

func TestConcurrentConfigUpload(t *testing.T) {
	configBlob := []byte("{}")
	layerBlob := []byte("layer data")
	configDigest := digest.FromBytes(configBlob)
	layerDigest := digest.FromBytes(layerBlob)

	// Create a V2 manifest
	m := manifest.Schema2{
		SchemaVersion: 2,
		MediaType:     manifest.DockerV2Schema2MediaType,
		ConfigDescriptor: manifest.Schema2Descriptor{
			MediaType: manifest.DockerV2Schema2ConfigMediaType,
			Size:      int64(len(configBlob)),
			Digest:    configDigest,
		},
		LayersDescriptors: []manifest.Schema2Descriptor{
			{
				MediaType: manifest.DockerV2Schema2LayerMediaType,
				Size:      int64(len(layerBlob)),
				Digest:    layerDigest,
			},
		},
	}
	manifestBlob, err := m.Serialize()
	assert.NoError(t, err)

	src := &mockImageSource{
		t:          t,
		configBlob: configBlob,
		layerBlob:  layerBlob,
		manifest:   manifestBlob,
	}
	dest := &mockImageDestination{
		t:                t,
		configUploaded:   make(chan struct{}),
		layerUploadBlock: make(chan struct{}),
	}

	srcRef := &mockImageReference{src: src}
	destRef := &mockImageReference{dest: dest}

	// Link references
	src.ref = srcRef
	dest.ref = destRef

	ctx := context.Background()
	opts := &Options{
		SourceCtx:      nil,
		DestinationCtx: nil,
	}

	go func() {
		defer close(dest.layerUploadBlock) // Ensure we don't hang if test fails
		// Wait for config to be uploaded
		select {
		case <-dest.configUploaded:
			// Success! Config uploaded while layer was blocked (we haven't unblocked it yet).
		case <-time.After(5 * time.Second):
			t.Error("Config upload timed out")
		}
	}()

	policyContext, err := signature.NewPolicyContext(&signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}})
	assert.NoError(t, err)

	_, err = Image(ctx, policyContext, destRef, srcRef, opts)
	assert.NoError(t, err)
}

// Ensure mockImageReference satisfies types.ImageReference
var _ types.ImageReference = &mockImageReference{}
// Ensure mockImageDestination satisfies types.ImageDestination (public)
// We need to implement the public interface methods too.
func (m *mockImageDestination) PutBlob(ctx context.Context, stream io.Reader, inputInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	// Map to our internal helper logic
	_, err := m.PutBlobWithOptions(ctx, stream, inputInfo, private.PutBlobOptions{IsConfig: isConfig})
	return inputInfo, err
}
