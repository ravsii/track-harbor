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
The top bar has no track dropdown: on load it shows the last played available
track, or the most recently added available one, without autoplay. Play starts the
shown track; individual tracks are selected from their Play actions in the library.

## Spacing baseline

Use half the previous spacing throughout the server UI. Keep these exact
values in sync with `internal/web/assets/theme.css`; halve padding, margins
and gaps, not text, icons or player control sizes:

| Area | Desktop | Narrow screens |
| --- | --- | --- |
| Sidebar padding | `8px 6px`; nav link `4px 5px`, gap `2px` | `4px 6px` at 950px; `4px 5px` at 700px |
| Top bar content padding | `6px clamp(12px, 2.5vw, 41px)` | `4px 10px 6px` at 950px |
| Tracks content padding | `4px 6px 8px` | `4px 5px 8px` at 700px |
| Spotify, VK, Downloads, Settings content padding | `7px clamp(8px, 1.5vw, 19px) 12px` | `6px 8px 10px` at 950px |
| Non-Tracks header / panel / list spacing | Header-to-panel `6px`; panel padding `9px`; panel-to-list `10px` | Same values |
| Track header / row padding (Tracks, Spotify, VK) | `0 5px 3.5px` / `3.5px 5px`; grid gap `4px` | Same compact spacing; tracks scroll horizontally if columns do not fit |
| Panel internals | Paragraph bottom `7px`; form gap `5px`; status top `8px` and padding top `5px` | Same values |
| Form controls | Input and textarea padding `5.5px 6px`; button padding `5.5px 8px`, with a `32px` minimum button height | Same values |
| Proxy list | Panel-to-panel `10px`; list top `6px`; row padding `5px 0`, row gap `4px 6px` | Same values |
| Download rows | Header padding `0 8px 5px`; row padding `6px 8px`, grid gap `6px` | Same values |

Tracks has no fixed content-width or track-list-height cap: its tracks fill the
space beside the sidebar and scroll with the page. Keep horizontal scrolling
only when the track columns cannot fit on a narrow screen. Preserve the top
player and source navigation rather than spending their space on decoration.
Keep the sidebar below the sticky player while scrolling; if its navigation
exceeds the available viewport height, scroll inside the sidebar. On narrow
screens, keep the source links in a horizontally scrollable row. Do not place
decorative or informational copy at the bottom of the sidebar.
The row number becomes Play on hover or keyboard focus; on touch, Play remains
visible. Center the number column heading over the numbers; leave the download
action column without a heading. Highlight the current track's title with an
accent background and dark text in both themes, including after navigation;
do not rely on the row background, which resembles hover. While it plays,
show a small accent-colored equalizer instead of its Play button in the number
column. Hover or visible keyboard focus reveals Pause in its place; without
hover, keep the equalizer visible. On pause, restore Play and hide the equalizer;
resuming the same track keeps its position. Do not highlight the
initial unplayed candidate. Show the duration of the local MP3 when available
and put its download
action at the right edge of each row. Missing files have neither action active.
Spotify, VK, and Downloads start with a compact page title rather than a large
decorative hero. Put connection, sync, and download controls directly beneath it.
When a source is connected, tuck the credential replacement form into a collapsed
disclosure so Sync and the track list remain close to the top. Keep that form
visible by default until the source is connected.
Settings uses the same compact panel and theme colors as the source pages.
Require an explicit user confirmation before any mutating or destructive
action (including removal), unless the request expressly says otherwise.
Settings form buttons fit their labels rather than stretching across the grid;
Show URL and Remove have matching icons on the left. The reveal icon changes
with its Show/Hide state.
Show personal proxies in their original URL format, for example
`socks5://********:********@host:port`, with exactly eight asterisks for
each existing username and password, regardless of length. Require
confirmation before deletion. Never render credentials in the initial HTML:
Show URL fetches and displays the original URL only in the row, where its text
can be selected and copied; Hide restores the mask. Do not add a second field.
Public proxies retain an empty replacement field, saved count and explicit
clearing control. The import button confirms before starting and reports its
background check progress and result alongside the saved count. Saving
settings must not interrupt the persistent player.

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
Place a button's icon to the left of its label when that fits the action
(for example, check, sync, save or start). Keep icon-only playback controls
unchanged.

Every existing and future visual element must work in both themes: text,
backgrounds, borders, buttons, forms, hover and focus states, errors,
indicators, and the player. Use semantic theme color variables rather than
hard-coded component colors. Check legibility and states in both themes when
adding or changing an element.
