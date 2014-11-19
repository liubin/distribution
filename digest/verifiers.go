package digest

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"hash"
	"io"
	"io/ioutil"

	"github.com/docker/docker/pkg/tarsum"
)

type Verifier interface {
	io.Writer

	// Verified will return true if the content written to Verifier matches
	// the digest.
	Verified() bool

	// Planned methods:
	// Err() error
	// Reset()
}

func DigestVerifier(d Digest) Verifier {
	alg := d.Algorithm()
	switch alg {
	case "md5", "sha1", "sha256":
		return hashVerifier{
			hash:   newHash(alg),
			digest: d,
		}
	default:
		// Assume we have a tarsum.
		version, err := tarsum.GetVersionFromTarsum(string(d))
		if err != nil {
			panic(err) // Always assume valid tarsum at this point.
		}

		pr, pw := io.Pipe()

		// TODO(stevvooe): We may actually want to ban the earlier versions of
		// tarsum. That decision may not be the place of the verifier.

		ts, err := tarsum.NewTarSum(pr, true, version)
		if err != nil {
			panic(err)
		}

		// TODO(sday): Ick! A goroutine per digest verification? We'll have to
		// get the tarsum library to export an io.Writer variant.
		go func() {
			io.Copy(ioutil.Discard, ts)
			pw.Close()
		}()

		return &tarsumVerifier{
			digest: d,
			ts:     ts,
			pr:     pr,
			pw:     pw,
		}
	}

	panic("unsupported digest: " + d)
}

// LengthVerifier returns a verifier that returns true when the number of read
// bytes equals the expected parameter.
func LengthVerifier(expected int64) Verifier {
	return &lengthVerifier{
		expected: expected,
	}
}

type lengthVerifier struct {
	expected int64 // expected bytes read
	len      int64 // bytes read
}

func (lv *lengthVerifier) Write(p []byte) (n int, err error) {
	n = len(p)
	lv.len += int64(n)
	return n, err
}

func (lv *lengthVerifier) Verified() bool {
	return lv.expected == lv.len
}

func newHash(name string) hash.Hash {
	switch name {
	case "sha256":
		return sha256.New()
	case "sha1":
		return sha1.New()
	case "md5":
		return md5.New()
	default:
		panic("unsupport algorithm: " + name)
	}
}

type hashVerifier struct {
	digest Digest
	hash   hash.Hash
}

func (hv hashVerifier) Write(p []byte) (n int, err error) {
	return hv.hash.Write(p)
}

func (hv hashVerifier) Verified() bool {
	return hv.digest == NewDigest(hv.digest.Algorithm(), hv.hash)
}

type tarsumVerifier struct {
	digest Digest
	ts     tarsum.TarSum
	pr     *io.PipeReader
	pw     *io.PipeWriter
}

func (tv *tarsumVerifier) Write(p []byte) (n int, err error) {
	return tv.pw.Write(p)
}

func (tv *tarsumVerifier) Verified() bool {
	return tv.digest == Digest(tv.ts.Sum(nil))
}
