# TrackHarbor Web Visual Design

## Interface priorities

The site's primary purpose is to make **downloading and listening to music**
easy. Download and Play actions should be clear and require as few steps as
possible; secondary elements should not distract from these tasks.

A top bar with a player must appear on **every page**. Users must be able to
start a saved track from the top bar regardless of the current page, control
playback, and see the currently playing track. Navigating between pages must
not stop playback or reset the track or playback position. A track's Play
action on any page must use the same player. The top bar and player must work
on narrow screens and in both color themes.

The top bar spans the viewport, with playback controls centered relative to
the full viewport: logo on the left, current track above the seek bar, and
the theme button on the far right. The shuffle button toggles between sequential
and random playback. The volume icon toggles mute without losing the
previous level. The theme button cycles through System, Light, and Dark.
The top bar has no track dropdown: Play starts the first saved track, and
individual tracks are selected from their Play actions in the library.

## Color palette

The primary visual direction is **marine graphite**. Use these colors in the
dark theme:

| Role | Color |
| --- | --- |
| Page background | `#0B1522` |
| Sidebar | `#101E2D` |
| Cards and panels | `#172638` |
| Primary text | `#EDF6F7` |
| Secondary text | `#AEC3CB` |
| Borders | `#315063` |
| Accent and active elements | `#5DD6D0` |
| Text on accent backgrounds | `#08292C` |
| Selected-item background | `#193D47` |

The light theme keeps **the same turquoise accent**. Only backgrounds,
surfaces, and text colors adapt:

| Role | Color |
| --- | --- |
| Page background | `#F4F2ED` |
| Sidebar | `#E9E8E5` |
| Cards and panels | `#FFFFFF` |
| Primary text | `#202633` |
| Secondary text | `#596376` |
| Borders | `#D4D8DF` |
| Accent and active elements | `#5DD6D0` |
| Text on accent backgrounds | `#08292C` |
| Selected-item background | `#DCF3F1` |

Use turquoise for button backgrounds, lines, and indicators in both themes.
Small turquoise text is hard to read on light backgrounds: use the primary
text color for links and accent labels, with turquoise underlines, borders,
or backgrounds. Text on turquoise buttons must always be dark.

The interface must offer **Light**, **Dark**, and **System** theme options.
System follows OS settings, including changes while the page is open.
Explicit choices are saved in the browser.

## Icons

Use the free [Hugeicons](https://hugeicons.com/) **Stroke Rounded** icons
throughout the server web interface instead of Unicode symbols or emoji for
navigation, playback, downloads, and status actions. Use the MIT-licensed
`@hugeicons/core-free-icons` set, not Pro assets. Keep the selected SVG icons
  self-hosted so the local server does not depend on an external CDN. Retain the
  MIT notice in `docs/hugeicons-license.txt`. Decorative icons must be
hidden from assistive technology; icon-only buttons must have descriptive
accessible labels. Icons inherit the surrounding text color and must work in
both themes.

Every existing and future visual element must work in both themes: text,
backgrounds, borders, buttons, forms, hover and focus states, errors,
indicators, and the player. Use semantic theme color variables rather than
hard-coded component colors. Check legibility and states in both themes when
adding or changing an element.
