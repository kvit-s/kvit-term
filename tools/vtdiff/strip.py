import sys
d = open(sys.argv[1], 'rb').read()
if d.startswith(b'Script started'):
    d = d[d.index(b'\n') + 1:]
i = d.rfind(b'\nScript done')
if i >= 0:
    d = d[:i]
open(sys.argv[2], 'wb').write(d)
