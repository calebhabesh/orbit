"""Small VT screen oracle for the renderer commands used in T09 PTY tests.

Keeps cells instead of searching byte deltas: a differential renderer may emit
only changed characters of a visible label. No snapshots are acceptance oracles.
"""
import codecs
import re
import unicodedata


class Screen:
    def __init__(self, width, height):
        self.width, self.height = width, height
        self.cells = [[" "]*width for _ in range(height)]
        self.x = self.y = 0
        self.top, self.bottom = 0, height-1
        self.autowrap = True
        self.saved = (0,0)
        self.last_char = " "
        self.pending = ""
        self.decoder = codecs.getincrementaldecoder("utf-8")("replace")

    def resize(self, width, height):
        self.cells = [(line+[" "]*width)[:width] for line in (self.cells+[[" "]*width for _ in range(height)])[:height]]
        self.width, self.height = width, height
        self.top, self.bottom = 0, height-1
        self.x, self.y = min(self.x,width-1), min(self.y,height-1)

    def text(self):
        return "\n".join("".join(line) for line in self.cells)

    def linefeed(self):
        if self.y == self.bottom:
            self.cells.pop(self.top)
            self.cells.insert(self.bottom,[" "]*self.width)
        else:
            self.y = min(self.height-1,self.y+1)


    def feed(self, data):
        self.pending += self.decoder.decode(data)
        while self.pending:
            c = self.pending[0]
            if c=="\x1b":
                if len(self.pending)<2:
                    return
                if self.pending[1]=="[":
                    match = re.match(r"\x1b\[([0-?]*)([ -/]*)([@-~])", self.pending)
                    if not match:
                        return
                    self.csi(match[1], match[3])
                    self.pending = self.pending[match.end():]
                    continue
                if self.pending[1]=="]":
                    match = re.match(r"\x1b\].*?(?:\x07|\x1b\\)", self.pending, re.S)
                    if not match:
                        return
                    self.pending = self.pending[match.end():]
                    continue
                if self.pending[1]=="M":
                    if self.y==self.top:
                        self.cells.insert(self.top,[" "]*self.width)
                        self.cells.pop(self.bottom+1)
                    else:
                        self.y -= 1
                if self.pending[1]=="7": self.saved=(self.x,self.y)
                if self.pending[1]=="8": self.x,self.y=self.saved
                self.pending = self.pending[2:]
                continue
            self.pending = self.pending[1:]
            if c=="\r": self.x=0
            elif c=="\n": self.linefeed()
            elif c=="\b": self.x=max(0,self.x-1)
            elif ord(c)<32: pass
            else: self.put(c)

    def put(self,c):
        if unicodedata.combining(c):
            at=max(0,min(self.x-1,self.width-1))
            if self.cells[self.y][at]=="" and at>0: at-=1
            self.cells[self.y][at]+=c
            return
        w=2 if unicodedata.east_asian_width(c) in ("W","F") else 1
        if self.x+w>self.width:
            if self.autowrap: self.x=0; self.linefeed()
            else: self.x=max(0,self.width-w)
        self.last_char=c
        self.cells[self.y][self.x]=c
        if w==2: self.cells[self.y][self.x+1]=""
        self.x+=w

    def csi(self, args, code):
        if args.startswith("?"):
            if "7" in args[1:].split(";") and code in "hl": self.autowrap=(code=="h")
            if "1049" in args[1:].split(";") and code=="h":
                self.cells=[[" "]*self.width for _ in range(self.height)];self.x=self.y=0
            return
        if any(c in args for c in "><="): return
        values = [int(s) if s else 0 for s in args.split(";")]
        n = values[0] or 1
        if code=="m": return
        if code=="r":
            self.top=max(0,n-1);self.bottom=min(self.height-1,(values[1] if len(values)>1 and values[1] else self.height)-1)
            self.x=self.y=0;return
        if code=="S":
            for _ in range(min(n,self.bottom-self.top+1)): self.cells.pop(self.top);self.cells.insert(self.bottom,[" "]*self.width)
            return
        if code=="T":
            for _ in range(min(n,self.bottom-self.top+1)): self.cells.insert(self.top,[" "]*self.width);self.cells.pop(self.bottom+1)
            return
        if code=="s": self.saved=(self.x,self.y);return
        if code=="u": self.x,self.y=self.saved;return
        if code=="b":
            for _ in range(n): self.put(self.last_char)
            return
        self.x = min(self.x,self.width-1)
        if code in "Hf":
            self.y = min(self.height-1,n-1)
            self.x = min(self.width-1,(values[1] if len(values)>1 and values[1] else 1)-1)
        elif code=="d": self.y=min(self.height-1,n-1)
        elif code=="G": self.x=min(self.width-1,n-1)
        elif code=="A": self.y=max(0,self.y-n)
        elif code=="B": self.y=min(self.height-1,self.y+n)
        elif code=="C": self.x=min(self.width-1,self.x+n)
        elif code=="D": self.x=max(0,self.x-n)
        elif code=="E": self.x=0; self.y=min(self.height-1,self.y+n)
        elif code=="F": self.x=0; self.y=max(0,self.y-n)
        elif code=="K":
            start,end = (self.x,self.width) if values[0]==0 else ((0,self.x+1) if values[0]==1 else (0,self.width))
            self.cells[self.y][start:end] = [" "]*(end-start)
        elif code=="J":
            if values[0]==2: self.cells=[[" "]*self.width for _ in range(self.height)]
            elif values[0]==0:
                self.cells[self.y][self.x:]=[" "]*(self.width-self.x)
                for i in range(self.y+1,self.height): self.cells[i]=[" "]*self.width
        elif code=="P": self.cells[self.y]=(self.cells[self.y][:self.x]+self.cells[self.y][self.x+n:]+[" "]*n)[:self.width]
        elif code=="@": self.cells[self.y]=(self.cells[self.y][:self.x]+[" "]*n+self.cells[self.y][self.x:])[:self.width]
        elif code=="X": self.cells[self.y][self.x:min(self.width,self.x+n)]=[" "]*min(n,self.width-self.x)
        elif code=="M":
            if self.top<=self.y<=self.bottom:
                count=min(n,self.bottom-self.y+1)
                for _ in range(count): self.cells.pop(self.y);self.cells.insert(self.bottom,[" "]*self.width)
        elif code=="L":
            if self.top<=self.y<=self.bottom:
                count=min(n,self.bottom-self.y+1)
                for _ in range(count): self.cells.insert(self.y,[" "]*self.width);self.cells.pop(self.bottom+1)
