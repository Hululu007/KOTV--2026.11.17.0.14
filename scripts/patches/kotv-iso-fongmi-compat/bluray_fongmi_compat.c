/* Compatibility symbols expected by FongMi/mpv (ada8002+) but not yet in
 * VideoLAN libbluray 1.4/1.5 releases. Implemented on top of the public API so
 * Blu-ray/ISO playback stays enabled. Remove once upstream ships these. */
#include <libbluray/bluray.h>

int bd_seek_time_checked(BLURAY *bd, uint64_t tick)
{
  if (!bd)
    return 0;
  return bd_seek_time(bd, tick) >= 0;
}

uint32_t bd_get_titles_with_menu_info(BLURAY *bd, uint8_t flags,
                                      uint32_t min_title_length,
                                      int *has_interactive_graphics)
{
  uint32_t n;
  if (has_interactive_graphics)
    *has_interactive_graphics = 0;
  if (!bd)
    return 0;
  n = bd_get_titles(bd, flags, min_title_length);
  if (has_interactive_graphics) {
    const BLURAY_DISC_INFO *di = bd_get_disc_info(bd);
    if (di) {
      *has_interactive_graphics =
          (di->num_bdj_titles > 0) ||
          (di->top_menu && di->top_menu->interactive) ||
          (di->first_play && di->first_play->interactive);
    }
  }
  return n;
}
