package audio

import "testing"

func TestAMutedUISetStaysSilentAndOpensNothing(t *testing.T) {
	u := OpenUI(true)
	u.Play(CueMove)
	u.Close()
	if u.ready || u.owns {
		t.Error("a muted set loaded sounds or took the device")
	}
	var none *UI
	none.Play(CueSelect) // a missing set is silent too
	none.Close()
	if !none.Muted() {
		t.Error("a missing set is not muted")
	}
}

func TestEveryCueHasASoundAndAVolume(t *testing.T) {
	pcm := cuePCM()
	for c := Cue(0); c < cueCount; c++ {
		if len(pcm[c]) == 0 {
			t.Errorf("cue %d is empty", c)
		}
		if cueVolume[c] <= 0 || cueVolume[c] > 1 {
			t.Errorf("cue %d has volume %v", c, cueVolume[c])
		}
	}
}

func TestPlayIgnoresACueThatDoesNotExist(t *testing.T) {
	u := OpenUI(false)
	u.Play(Cue(-1))
	u.Play(cueCount)
	if u.ready {
		t.Error("an unknown cue loaded the sounds")
	}
}
