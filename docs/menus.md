# Menus

The design for autobahn's menus, as locked in the interactive prototype
(`Prototype — locked` on the menu canvas), and as built. This was the brief the
build worked from; where what was built differs, it says so. Where it and the prototype disagree, the rules here win; the prototype's
numbers for anything the game already knows (cars, prices, ghosts) are copied
from the code and may drift.

The prototype has buttons that exist only so it can be tried on a phone: the
**Esc / back** button, the **Pause** button on the drive, **+bounty /
+earnings / Reset**, and **Player 2 joins (demo)** in the lobby. None of them
are part of the game.

## Principles

* **One look: Signpost.** Dark ground, white type, one blue accent
  (`menuSelected`, #68ACE8) for what is selected. Green (#7BD389) means
  yours or done, amber (#F0B070) means for sale, next, or a warning, and grey
  means locked. Every screen uses those meanings and no others.
* **Keyboard first, mouse too.** ↑↓ moves between rows, ←→ changes the row,
  enter selects, esc goes back one screen. The mouse does the same (see "Mouse
  and sound"). Every screen keeps one selection and
  draws it with the accent: a filled row, an outlined card or a ringed
  control. The bottom bar says which keys do what, as it does now.
* **Locked is shown, never hidden.** Maps, cars, packs and paints the player
  does not hold appear greyed, with a padlock and how far there is to go.
  The menus never let a locked thing start, and say why when asked.
* **A row says what it is and what it is set to, separately.** "Map: city"
  becomes a card named City with a season tag beside it. The season is never
  part of a name.
* **What banks is always said.** Anything that will not bank (the autopilot,
  a custom map, a challenge) says so where it is chosen and on the drive.
* **Type is a bundled TTF.** Overpass (SIL OFL, `internal/game/fonts/OFL.txt`),
  embedded with `go:embed` and loaded with `rl.LoadFontFromMemory`, in regular,
  bold and extra bold (`typeFace`'s `weightRegular`, `weightBold` and
  `weightHeavy`). Each weight and pixel size is rasterised once, on first use,
  at exactly that size, so the type is crisp at one to one. The default raylib
  face stays for the in-drive HUD, and is the fallback when there is no window
  or a font will not load. `typeFace` in `signpost.go` and `menufont.go` are the
  only places the font is known. Available characters: ASCII, the pound sign,
  middle dot, single angle quotes, multiplication and minus signs, bullet,
  ellipsis, en and em dashes, curly quotes, the degree sign and the four
  arrows. Anything else would draw as a box, so add it to `menuGlyphs` (and
  check the font has it: a test does).

## Behind the menus: the menu drive

On by default, off under Settings › Menus. While on, the autopilot drives the
map chosen under Play behind every menu screen except the garage and pause.

* It is a real `Game` in an attract mode: autopilot on, no HUD, no AI camera
  panel, no police, light traffic, quiet audio. Nothing it does is collected:
  it never passes through `Shell.collect`, so it banks no bounty, earnings or
  achievements.
* Changing the map under Play rebuilds it, once the choice has settled for a
  moment so stepping through maps does not build each one. Choosing the
  Custom card does not change it: the drive shows the map under Play's cards
  until a custom map is being hosted, and only then, in the lobby, drives that
  map's share code (`Shell.customPlay`, which the title page clears).
* The menus darken only the left of the screen, where the list is, and fade
  to clear by three quarters of the way across. While the drive is on, the
  title screen's card drops its picture and sits small at the bottom right.
* Off, every screen stands on the plain backdrop (`menuBackdrop`) and the
  title card shows the map's picture. The picture is the custom map page's
  plan of the map (`drawPlanPicture`): a patch of it built by the generator,
  so it is the real place.
* It is drawn through the scene shader like any drive, so the vision rules
  in CLAUDE.md hold for it unchanged. The menus are drawn after it, in 2D.

## The screens

```
Title ─┬─ Play ─┬─ (Free drive) ── Custom map
       │        ├─ (Challenge)
       │        └─ (Multiplayer) ── Lobby
       ├─ Garage
       ├─ Career
       ├─ Settings
       └─ Quit ── dialog
Drive ── esc ── Pause ─┬─ Garage
                       ├─ Settings
                       └─ (multiplayer: esc ── Leave dialog)
```

### Title

The list on the left, grouped:

| Group | Row      | Says beside it                    |
|-------|----------|-----------------------------------|
| PLAY  | Play     | the mode and map last chosen      |
| YOU   | Garage   | car · paint                       |
|       | Career   | NEW when a pack has just unlocked |
|       | Settings |                                   |
|       | Quit     |                                   |

The card on the right follows the selection: the chosen map for Play, the car
for Garage, bounty and the next pack for Career. Esc opens the quit dialog.

The mode last chosen under Play is stored (`Settings.PlayMode`, with the
challenge in `Settings.Challenge`), so the Play row says "free drive, city",
"challenge, <its name>", "multiplayer, host chase" or "multiplayer, join
<address>", and Play opens where it was left. Whether multiplayer hosts or
joins, and the address, last only as long as the game is open.

Removed from the title: the map row (it is in Play), Host and Join a chase
(Play › Multiplayer), Challenges (Play › Challenge), Custom map (Play › Free
drive › Custom), Shop (the garage), Packs and Achievements (Career).

### Play

One screen for everything that starts a drive. Along the top, the title and
a three-way switch: **Free drive · Challenge · Multiplayer**. Along the
bottom, a bar with Back, a one-line summary of what will start, the reason it
cannot when it cannot, and the big start button.

**Free drive**

* WHERE: five cards across the width: City, Seatown, Village, Roadtown,
  Custom. Each has its season tag. A locked map is greyed, with a padlock,
  bounty banked against its pack's price and a bar. Custom shows its share
  code and "banks nothing".
* HOW: controls in a grid:
  * Who drives: You / Autopilot.
  * Autopilot style (when the autopilot drives): normal, cautious, brisk,
    aggressive, getaway. This is `Settings.Mode`.
  * Weather: drift, clear, rain, fog, storm (`Settings.Weather`).
  * Start time: day, dusk, night, dawn (`Settings.Time`).
  * Car: opens the garage.
  * Edit the custom map (when Custom is chosen): opens the custom map screen.
* Start: **Drive** or **Watch the autopilot**. Disabled on a locked map.

**Challenge**

* The challenges (`scenario.All`) as cards, four across: map and season, name,
  what the goal is, the ghost's score and the player's best.
* Car, and **Start the challenge**. Banks nothing; F5 retries on the drive.

**Multiplayer**

* Two panels side by side, **Host a game** and **Join a game**; the one
  chosen is lit and the other dimmed.
* Host: the game (Chase, Co-op: two cars, Co-op: one car, which is
  `hostMode`), the map (the same five, locked ones greyed), the car.
  **Start hosting**. A custom map is hosted like any other: the joiner is sent
  its share code in the snapshot, as it always was, and builds the same
  world. It banks nothing, and the summary says so.
* Join: the host's address in a text field, port 7777 assumed. **Connect**,
  disabled while the field is empty.

### Custom map

Sliders on the left, a plan of the map on the right.

* Along the top of the left side, sections: **Start · Layout · Land · Rules ·
  Share**. The section shown is the one the selection is in, so ↑↓ walks
  through every row and the section follows.
  * Start: starts from (held maps only), seed, coast, weather.
  * Layout: shortest block, longest block, main road every, roundabouts,
    open ground, building height.
  * Land: winding, hills, hill length.
  * Rules: speed limits, season, traffic mix, road grip.
  * Share: copy the code, paste a code, save it by name, allow unsupported.
* Numbers are bars with − and +; choices (limits, season, traffic mix,
  weather) are a row of options; on/off is a switch. A value past the
  measured range turns amber and the map is tagged UNSUPPORTED.
* The plan is drawn from the spec as it stands: blocks, main roads, winding,
  roundabouts, open ground, hills and the coast, tinted for the season. It
  is drawn from `custom.Spec.Params` through the generator on a small patch
  of ground, not invented, so it shows what will be driven.
* Under the plan: the share code, "banks nothing", and **Drive this map**.

Everything in `custom.Clamp`, the ranges file and the share code is
unchanged. This is a new face on `customScreen`.

### Lobby

A card over the dimmed menu drive.

* A spinner and **HOSTING** or **CONNECTING**, with what it is waiting for.
* The address: for the host, the one to tell the other player, with Copy;
  for a joiner, the one being reached.
* Two player slots: you, ready, with your car and paint; the other player,
  waiting until they arrive.
* The mode and map, and **Cancel** (esc).

The game starts when the session is peered, as it does now. The lobby opens
only once hosting or joining has started; if it cannot start, or the session
fails while waiting, the game goes back to the title with the reason. Cancel
goes back to Play.

### Garage

Its own place: entering it leaves the menu drive or the paused game for a
garage, the car on the existing turntable (`showroom`), and fades in from
black.

* The stage, top left: the car large, a tag (DRIVING, YOURS, FOR SALE,
  LOCKED), its place in the line-up, the line saying how it drives, its name
  and pack, and four bars: top speed, acceleration, grip and weight. They come from `sim.PlayerBodyOf`,
  and a tick marks the car being driven now so any other can be compared.
* The panel, right: paint as a grid of swatches (unowned ones faded); below
  it, buying. A car or paint not owned shows **Buy … · price**, greyed when
  earnings fall short; a locked car says which pack and how much bounty
  opens it. **Done** at the foot.
* The cars, along the bottom: every player car as a card in one line, seven
  at a time with the selection kept in view, each with its status. Above
  them, a filter: everything, yours, for sale.
* ↑↓ moves between cars, paint, the buy buttons, the filter and Done; ←→
  browses. A car is only driven once it is owned.

The shop screen goes. Its rules (`shop.Holdings`, what is for sale once its
pack is held, paints that are never sold) are unchanged.

In a chase against another person the car's shape is fixed for the chase, as
now, and the garage says so.

### Pause

Over the paused scene, dimmed.

* Top line: PAUSED, the map and season, the mode, and a tag that says
  whether the drive is banking.
* Six tiles across the width: time, distance, bounty so far (the ghost's
  score in a challenge), wanted, faults, top speed. They come from
  `Game.Summary` and the bounty meter.
* Left: **Resume**, **Restart**, **Autopilot** (a switch, free drive only;
  the same as Tab while driving), **Garage**, **All settings**, and, apart
  and in red, **Abandon the drive**.
* Right: quick settings (viewpoint, AI camera, box labels, volume), and a
  card about this drive: the bounty so far and what abandoning or restarting
  does to it; in a challenge, you against the ghost; or why nothing banks.

In a networked game esc still leaves rather than pausing, but through the
leave dialog, not at once. The game runs on under the dialog, so the other
player's neither freezes nor times out: the shell steps it without drawing
(`Game.advanceHandsOff`) while the dialog has the keys, and draws it behind
the dialog. Nothing is pressed meanwhile, so a car driven by hand coasts and
an autopilot drives on.

Every step of the shell draws exactly one frame, of whatever is showing once
the step is done (`Shell.Step`, `drawScreen`), even when esc, a page's Back or
F5 changed the screen. raylib reads the keys only when a frame ends, so a step
that drew nothing would see its own esc again on the next. Raylib's exit key
is turned off only for the menus (`shell`); a run from the command line keeps
it, and esc closes that window.

The pause page notes the top speed with `Game.notePeakSpeed` after every step
of the drive.

### Career

One page.

* Three tiles: bounty banked, earnings, the next pack and how much is left
  to bank for it.
* Left: the packs as a route, in price order. Each has a dot (held, next,
  locked), its maps and cars, and how much bounty is left with a bar.
* Right: the achievements as a grid of tiles. Earned ones are ticked;
  hidden ones show ??? until earned. Progress, where an achievement has it,
  is shown on its tile.

### Settings

Sections on the left, the chosen section's settings on the right, each with
a line saying what it does and a real control.

| Section   | Setting      | Control | Stored as          |
|-----------|--------------|---------|--------------------|
| Menus     | Menu drive   | switch  | new, default on    |
| Sound     | Volume       | bar     | `Volume`           |
|           | Sound        | switch  | `Muted` (inverted) |
| View      | Viewpoint    | options | `Camera`           |
|           | AI camera    | switch  | `ShowPanel`        |
|           | Box labels   | switch  | `ShowLabels`       |
| Sound     | Menu sounds  | switch  | `NoMenuSounds` (inverted) |
| Display   | Full screen  | switch  | `Fullscreen`       |
|           | Window size  | options | `WindowW`, `WindowH` |
|           | VSync        | switch  | `VSync`            |
| World     | Traffic      | bar     | `Traffic`          |
|           | Police       | bar     | `Police`           |
|           | Day length   | options | `DayLength`        |
| Autopilot | Jobs on auto | switch  | `AutopilotJobs`    |

Weather, start time and autopilot style move to Play, where they are chosen
for a drive. They stay in `Settings` and are stored as before.

Settings are saved when the screen closes, as now.

### Quit and leave

A dialog over the dimmed screen behind it, with **Stay** (selected) and
**Quit** (red).

* Quit: "Your career is saved", with the bounty and earnings.
* Leave, in multiplayer: "There is no pausing with someone else in it.
  Leaving ends the game for both of you."

## Mouse and sound

**The mouse** reads the same intents the keys do (`pointer.go`). A page says
where each thing that can be chosen is as it draws (`hitRow`, `hitChoice`,
`hitDo`), and the next step turns the mouse into the `navInput` the keys make,
so every page takes it without knowing. Moving over a box selects its row, a
left click is enter (on a bar or a stepper, ← or → by the half clicked), a box
that is one choice of an options row (a card, a pill) picks that choice, the
wheel is ↑↓ and a right click is esc. A new page gets this by drawing its rows
with `drawRowView`, or by registering its own boxes.

**Sound.** The menus tick as the selection moves, chime when something is
chosen, fall when going back and thud when a locked row refuses (`audio.UI`).
They come from the key handling in `rowList.Update`, so the mouse and the keys
sound alike. The set opens the audio device itself when no drive has, and a
drive that closes it from under it makes it load them again. Settings › Sound
turns them off; muting turns off everything.

**Display.** Full screen is a borderless window, the window size is a few
presets that fit the screen and never go under 1280 by 720, and VSync holds the
frame rate to the screen's. They are applied to the open window as soon as they
change (`Shell.applyDisplay`), and on start. A size on the command line wins
over the stored one.

## Building it

In stages, each one a commit that passes `just ci`:

1. The menu model: rows that carry a key, a kind (action, switch, options,
   bar, stepper), their options and a state, built on crucible's
   `menu.Menu` for movement so the family's input stays the same. Tests for
   movement, adjusting and what each kind does.
2. The font, the colours and the Signpost drawing; the title screen and the
   quit dialog.
3. Play, all three modes, with the map cards and locks. The old title rows
   go.
4. The garage with the shop folded in. The shop screen goes.
5. Pause and the leave dialog.
6. Career. The packs and achievements screens go.
7. Settings in sections.
8. The custom map screen and its plan.
9. The lobby card.
10. The menu drive, and the garage leaving it for its own place.
11. Integration: the shell opens on the title page, every way back ends there,
    and the old screens are gone. The page ids are the whole list of screens
    now, with the drive (`screenPlaying`) the only screen that is not a page.

Built as planned.

The menus stay display-free where they can: what a screen shows is worked
out apart from how it is drawn, as `customScreen` already is, so the tests
need no window.

## Still to decide

Nothing open. Settled:

* What the menu drive costs. Measured (#50): perception is most of it, and at
  five looks a second it is about two milliseconds a frame, with light traffic
  and no police. It stays on by default; Settings turns it off.
* Mouse support. Added later: see "Mouse and sound".
* The garage is the existing turntable on a garage backdrop, drawn by the
  page itself on the plain backdrop, faded in from black.
