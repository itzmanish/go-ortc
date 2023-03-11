package buffer

import (
	"io"
	"sync"

	"github.com/pion/transport/v2/packetio"

	"github.com/livekit/mediatransportutil/pkg/bucket"
)

type FactoryOfBufferFactory struct {
	videoPool *sync.Pool
	audioPool *sync.Pool
}

func NewFactoryOfBufferFactory(trackingPackets int) *FactoryOfBufferFactory {
	return &FactoryOfBufferFactory{
		videoPool: &sync.Pool{
			New: func() interface{} {
				b := make([]byte, trackingPackets*bucket.MaxPktSize)
				return &b
			},
		},
		audioPool: &sync.Pool{
			New: func() interface{} {
				b := make([]byte, bucket.MaxPktSize*200)
				return &b
			},
		},
	}
}

func (f *FactoryOfBufferFactory) CreateBufferFactory() *Factory {
	return &Factory{
		videoPool:   f.videoPool,
		audioPool:   f.audioPool,
		rtpBuffers:  make(map[uint32]*Buffer),
		rtcpReaders: make(map[uint32]*RTCPReader),
	}
}

type Factory struct {
	sync.RWMutex
	videoPool   *sync.Pool
	audioPool   *sync.Pool
	rtpBuffers  map[uint32]*Buffer
	rtcpReaders map[uint32]*RTCPReader
}

func (f *Factory) GetOrNew(packetType packetio.BufferPacketType, ssrc uint32) io.ReadWriteCloser {
	f.Lock()
	defer f.Unlock()
	switch packetType {
	case packetio.RTCPBufferPacket:
		if reader, ok := f.rtcpReaders[ssrc]; ok {
			return reader
		}
		reader := NewRTCPReader(ssrc)
		f.rtcpReaders[ssrc] = reader
		reader.OnClose(func() {
			f.Lock()
			delete(f.rtcpReaders, ssrc)
			f.Unlock()
		})
		return reader
	case packetio.RTPBufferPacket:
		if reader, ok := f.rtpBuffers[ssrc]; ok {
			return reader
		}
		buffer := NewBuffer(ssrc, f.videoPool, f.audioPool)
		f.rtpBuffers[ssrc] = buffer
		buffer.OnClose(func() {
			f.Lock()
			delete(f.rtpBuffers, ssrc)
			f.Unlock()
		})
		return buffer
	}
	return nil
}

func (f *Factory) GetBufferPair(ssrc uint32) (*Buffer, *RTCPReader) {
	f.RLock()
	defer f.RUnlock()
	return f.rtpBuffers[ssrc], f.rtcpReaders[ssrc]
}

func (f *Factory) GetBuffer(ssrc uint32) *Buffer {
	f.RLock()
	defer f.RUnlock()
	return f.rtpBuffers[ssrc]
}

func (f *Factory) GetRTCPReader(ssrc uint32) *RTCPReader {
	f.RLock()
	defer f.RUnlock()
	return f.rtcpReaders[ssrc]
}
