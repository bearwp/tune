package main

import (
	"os"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

// Player wraps beep to provide a tiny, seekable MP3 player.
type Player struct {
	streamer beep.StreamSeekCloser
	ctrl     *beep.Ctrl
	volume   *effects.Volume
	format   beep.Format
	playing  bool
	vol      float64
}

func NewPlayer() *Player {
	return &Player{}
}

// InitSpeaker prepares the audio device once. Safe to call repeatedly.
func InitSpeaker() error {
	return speaker.Init(beep.SampleRate(44100), beep.SampleRate(44100).N(time.Second/10))
}

func (p *Player) Load(path string) error {
	p.Stop()

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	streamer, format, err := mp3.Decode(f)
	if err != nil {
		f.Close()
		return err
	}

	p.streamer = streamer
	p.format = format
	p.volume = &effects.Volume{Streamer: streamer, Base: 2, Volume: p.vol}
	p.ctrl = &beep.Ctrl{Streamer: p.volume, Paused: true}
	p.playing = false

	speaker.Play(p.ctrl)
	return nil
}

func (p *Player) Play() {
	if p.ctrl == nil {
		return
	}
	p.ctrl.Paused = false
	p.playing = true
}

func (p *Player) Pause() {
	if p.ctrl == nil {
		return
	}
	p.ctrl.Paused = true
	p.playing = false
}

func (p *Player) Toggle() {
	if p.playing {
		p.Pause()
	} else {
		p.Play()
	}
}

func (p *Player) IsPlaying() bool { return p.playing }

func (p *Player) Stop() {
	speaker.Clear()
	if p.streamer != nil {
		p.streamer.Close()
	}
	p.streamer = nil
	p.ctrl = nil
	p.playing = false
}

func (p *Player) Seek(d time.Duration) {
	if p.streamer == nil {
		return
	}
	target := p.streamer.Position() + p.format.SampleRate.N(d)
	if target < 0 {
		target = 0
	}
	if ln := p.streamer.Len(); target > ln {
		target = ln
	}
	p.streamer.Seek(target)
}

func (p *Player) Position() time.Duration {
	if p.streamer == nil {
		return 0
	}
	return p.format.SampleRate.D(p.streamer.Position())
}

func (p *Player) Length() time.Duration {
	if p.streamer == nil {
		return 0
	}
	return p.format.SampleRate.D(p.streamer.Len())
}

func (p *Player) VolumeUp() {
	if p.vol < 5 {
		p.vol += 0.5
	}
	if p.volume != nil {
		p.volume.Volume = p.vol
	}
}

func (p *Player) VolumeDown() {
	if p.vol > -5 {
		p.vol -= 0.5
	}
	if p.volume != nil {
		p.volume.Volume = p.vol
	}
}

func (p *Player) Volume() float64 { return p.vol }

// MP3Duration returns the duration of an MP3 file without playing it.
func MP3Duration(path string) time.Duration {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	s, format, err := mp3.Decode(f)
	if err != nil {
		return 0
	}
	defer s.Close()
	return format.SampleRate.D(s.Len())
}
