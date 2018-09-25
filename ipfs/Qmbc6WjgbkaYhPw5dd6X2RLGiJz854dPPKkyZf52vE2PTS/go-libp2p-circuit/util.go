package relay

import (
	"encoding/binary"
	"errors"
	"io"

	pb "gx/ipfs/Qmbc6WjgbkaYhPw5dd6X2RLGiJz854dPPKkyZf52vE2PTS/go-libp2p-circuit/pb"

	mh "gx/ipfs/QmPnFwZ2JXKnXgMw8CdBPxn7FWh6LLdjUjxV1fKHuJnkr8/go-multihash"
	peer "gx/ipfs/QmQsErDt8Qgw1XrsXf2BpEzDgGWtB1YLsTAARBup5b6B9W/go-libp2p-peer"
	ma "gx/ipfs/QmYmsdtJ3HsodkePE3eU3TsCaP2YvPZJ4LoXnNkDE5Tpt7/go-multiaddr"
	ggio "gx/ipfs/QmdxUuburamoF6zF9qjeQC4WYcWGbWuRmdLacMEsW8ioD8/gogo-protobuf/io"
	proto "gx/ipfs/QmdxUuburamoF6zF9qjeQC4WYcWGbWuRmdLacMEsW8ioD8/gogo-protobuf/proto"
	pstore "gx/ipfs/QmeKD8YT7887Xu6Z86iZmpYNxrLogJexqxEugSmaf14k64/go-libp2p-peerstore"
)

func peerToPeerInfo(p *pb.CircuitRelay_Peer) (pstore.PeerInfo, error) {
	if p == nil {
		return pstore.PeerInfo{}, errors.New("nil peer")
	}

	h, err := mh.Cast(p.Id)
	if err != nil {
		return pstore.PeerInfo{}, err
	}

	addrs := make([]ma.Multiaddr, 0, len(p.Addrs))
	for _, addrBytes := range p.Addrs {
		a, err := ma.NewMultiaddrBytes(addrBytes)
		if err == nil {
			addrs = append(addrs, a)
		}
	}

	return pstore.PeerInfo{ID: peer.ID(h), Addrs: addrs}, nil
}

func peerInfoToPeer(pi pstore.PeerInfo) *pb.CircuitRelay_Peer {
	addrs := make([][]byte, len(pi.Addrs))
	for i, addr := range pi.Addrs {
		addrs[i] = addr.Bytes()
	}

	p := new(pb.CircuitRelay_Peer)
	p.Id = []byte(pi.ID)
	p.Addrs = addrs

	return p
}

type delimitedReader struct {
	r   io.Reader
	buf []byte
}

// The gogo protobuf NewDelimitedReader is buffered, which may eat up stream data.
// So we need to implement a compatible delimited reader that reads unbuffered.
// There is a slowdown from unbuffered reading: when reading the message
// it can take multiple single byte Reads to read the length and another Read
// to read the message payload.
// However, this is not critical performance degradation as
// - the reader is utilized to read one (dialer, stop) or two messages (hop) during
//   the handshake, so it's a drop in the water for the connection lifetime.
// - messages are small (max 4k) and the length fits in a couple of bytes,
//   so overall we have at most three reads per message.
func newDelimitedReader(r io.Reader, maxSize int) *delimitedReader {
	return &delimitedReader{r: r, buf: make([]byte, maxSize)}
}

func (d *delimitedReader) ReadByte() (byte, error) {
	buf := d.buf[:1]
	_, err := d.r.Read(buf)
	return buf[0], err
}

func (d *delimitedReader) ReadMsg(msg proto.Message) error {
	mlen, err := binary.ReadUvarint(d)
	if err != nil {
		return err
	}

	if uint64(len(d.buf)) < mlen {
		return errors.New("Message too large")
	}

	buf := d.buf[:mlen]
	_, err = io.ReadFull(d.r, buf)
	if err != nil {
		return err
	}

	return proto.Unmarshal(buf, msg)
}

func newDelimitedWriter(w io.Writer) ggio.WriteCloser {
	return ggio.NewDelimitedWriter(w)
}
