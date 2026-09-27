# Deterministic escape-sequence streams for differential testing of terminal
# emulators at 80x24. synth-sections exercises one feature per section;
# synth-randN mixes the same operations at random.
import random, sys

E = '\x1b'
C = E + '['
words = ['alpha', 'beta', 'gamma', 'delta', 'ÄÖÜ', 'naïve', '中文字', '日本語', 'é', 'ẍy', '→', '😀', '█', 'tab\there']

def sgr_random(r):
    c = r.choice([
        '0', '1', '3', '4', '7', '9', '22', '23', '24', '27', '29',
        str(r.randint(30, 37)), str(r.randint(40, 47)), str(r.randint(90, 97)), str(r.randint(100, 107)),
        '38;5;%d' % r.randint(0, 255), '48;5;%d' % r.randint(0, 255),
        '38;2;%d;%d;%d' % (r.randint(0, 255), r.randint(0, 255), r.randint(0, 255)),
        '48;2;%d;%d;%d' % (r.randint(0, 255), r.randint(0, 255), r.randint(0, 255)), '39', '49'])
    return C + c + 'm'

def op_random(r):
    k = r.randrange(24)
    if k < 6: return r.choice(words) + ' '
    if k == 6: return sgr_random(r)
    if k == 7: return C + '%d;%dH' % (r.randint(1, 24), r.randint(1, 80))
    if k == 8: return C + str(r.randint(1, 5)) + r.choice('ABCD')
    if k == 9: return C + str(r.randint(0, 2)) + r.choice('JK')
    if k == 10: return C + str(r.randint(1, 6)) + r.choice('@PXLM')
    if k == 11: return C + '%d;%dr' % (r.randint(1, 10), r.randint(12, 24))
    if k == 12: return r.choice([E + 'D', E + 'M', E + 'E', '\r\n', '\n', '\r', '\b', '\t'])
    if k == 13: return C + str(r.randint(1, 4)) + r.choice('ST')
    if k == 14: return r.choice([E + '7', E + '8', C + 's', C + 'u'])
    if k == 15: return C + str(r.randint(1, 80)) + 'G'
    if k == 16: return C + str(r.randint(1, 24)) + 'd'
    if k == 17: return C + '?7' + r.choice('hl')
    if k == 18: return C + '4' + r.choice('hl')
    if k == 19: return E + '(0' + 'lqqkx' + E + '(B'
    if k == 20: return 'Z' + C + str(r.randint(1, 9)) + 'b'
    if k == 21: return E + ']0;title\x07' + E + ']133;A\x1b\\' + E + 'P1$r0m\x1b\\'
    if k == 22: return C + '?1049' + r.choice('hl')
    return r.choice(words) * r.randint(1, 8)

def sections():
    s = []
    s.append(C + '2J' + C + 'H')
    # DECALN fills the screen with E; erase all but the last rows
    s.append(E + '#8' + C + '22;1H' + C + '1J' + C + 'H')
    # colours and attributes
    for i, a in enumerate(['1', '3', '4', '7', '9', '1;3;4', '31', '42', '93', '104', '38;5;202', '48;5;17', '38;2;255;128;0', '48;2;0;64;128']):
        s.append(C + a + 'm' + 'sgr%d' % i + C + '0m ')
    s.append('\r\n')
    # wrap: fill a line exactly, then more text (pending wrap)
    s.append('W' * 80 + 'X\r\n')
    s.append('Y' * 79 + '中' + 'after-wide\r\n')
    # autowrap off
    s.append(C + '?7l' + 'N' * 90 + C + '?7h\r\n')
    # tabs and tab stops
    s.append('a\tb\tc\r\n' + C + '3g' + E + 'H' + C + '20G' + E + 'H\r' + '1\t2\t3\r\n' + E + 'c' * 0)
    # insert / delete characters
    s.append('0123456789' + C + '5D' + C + '2@' + '__' + C + '3P' + '\r\n')
    # insert mode
    s.append('abcdef' + C + '3D' + C + '4h' + 'XY' + C + '4l' + '\r\n')
    # erase variants
    s.append('erase-me-please' + C + '5D' + C + '0K' + '\r\n' + 'keep' + C + '1K' + 'z\r\n' + 'ech-test' + C + '4D' + C + '2X\r\n')
    # scroll region with index and reverse index
    s.append(C + '15;18r' + C + '15;1H' + 'r1\r\nr2\r\nr3\r\nr4\r\nr5\r\nr6' + E + 'M' * 3 + 'rev' + C + 'r')
    # insert / delete lines
    s.append(C + '10;1H' + 'L10' + C + '11;1H' + 'L11' + C + '10;1H' + C + '1L' + C + '12;1H' + C + '1M')
    # DEC line drawing
    s.append(C + '20;1H' + E + '(0' + 'lqqqqk\r\nx    x\r\nmqqqqj' + E + '(B')
    # repeat, combining, wide at the margin
    s.append(C + '23;1H' + '=' + C + '10b' + ' é à́ ' + C + '23;79H' + '字')
    # save/restore with attributes
    s.append(C + '1;60H' + C + '1;35m' + E + '7' + C + '0m' + C + '3;3H' + 'plain' + E + '8' + 'restored')
    # ignored strings
    s.append(E + ']0;window title\x07' + E + ']7;file:///tmp\x1b\\' + E + ']133;C\x07' + E + 'P+q544e\x1b\\' + C + '>1u' + C + '?2026h' + C + '?2026l')
    # alternate screen round trip keeps the main screen
    s.append(C + '?1049h' + 'on the alternate screen' + C + '?1049l')
    return ''.join(s)

out = sys.argv[1]
open(out + '/synth-sections.bin', 'wb').write(sections().encode('utf-8'))
for seed in range(1, 6):
    r = random.Random(seed)
    open(out + '/synth-rand%d.bin' % seed, 'wb').write(''.join(op_random(r) for _ in range(3000)).encode('utf-8'))
print('written')
