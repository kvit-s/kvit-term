#include "kvitterm/palette.h"

namespace kvitterm {

QColor Palette::resolve(const Color &color, bool asBackground) const
{
    switch (color.kind) {
    case Color::Default:
        return asBackground ? background : foreground;
    case Color::Indexed:
        return indexed(color.index);
    case Color::Rgb:
        return QColor(color.red, color.green, color.blue);
    }
    return asBackground ? background : foreground;
}

QColor Palette::indexed(int index) const
{
    if (index < 0)
        return foreground;
    if (index < 16)
        return ansi[index];
    if (index < 232) {
        // The 6x6x6 colour cube. The steps are not evenly spaced: the first is
        // black and the rest run from 95 to 255, which is what every terminal
        // has done since xterm chose it.
        static const int steps[6] = {0, 95, 135, 175, 215, 255};
        const int offset = index - 16;
        return QColor(steps[(offset / 36) % 6], steps[(offset / 6) % 6], steps[offset % 6]);
    }
    if (index < 256) {
        const int level = 8 + (index - 232) * 10;   // a 24-step grey ramp
        return QColor(level, level, level);
    }
    return foreground;
}

} // namespace kvitterm
