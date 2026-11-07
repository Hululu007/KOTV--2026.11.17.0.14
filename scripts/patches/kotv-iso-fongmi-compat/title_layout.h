/* FongMi/mpv (ada8002+) expects <dvdnav/title_layout.h>. Stock VideoLAN
 * libdvdnav 7.0.0 does not ship it yet; keep DVD/ISO enabled via this
 * compatibility header. Remove once upstream publishes the same API. */
#ifndef KOTV_DVDNAV_TITLE_LAYOUT_H
#define KOTV_DVDNAV_TITLE_LAYOUT_H

#include <dvdnav/dvdnav.h>
#include <dvdnav/dvd_types.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#ifndef DVDNAV_HAVE_TITLE_LAYOUT
#define DVDNAV_HAVE_TITLE_LAYOUT 1
#endif

DVDNAV_API dvdnav_status_t dvdnav_get_next_block_with_wait(dvdnav_t *self,
                                                          uint8_t *buf,
                                                          int32_t *event,
                                                          int32_t *len);

DVDNAV_API int8_t dvdnav_get_active_logical_stream(dvdnav_t *self,
                                                   dvdnav_stream_type_t type);

DVDNAV_API int8_t dvdnav_get_number_of_stream_attributes(
    dvdnav_t *self, dvdnav_stream_type_t type);

#ifdef __cplusplus
}
#endif

#endif /* KOTV_DVDNAV_TITLE_LAYOUT_H */
