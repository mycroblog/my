# agg.py: read `go tool pprof -traces` on stdin; print the bytes by library and by who asked for them.
import sys,re,collections
STD=re.compile(r'^(runtime|reflect|regexp|strings|bytes|sync|encoding|net|html|text|crypto|internal|os|io|fmt|sort|slices|maps|strconv|unicode|time|math|bufio|compress|context|errors|embed|database|log|mime|path|hash|iter|unique|weak|container|expvar|go|syscall|vendor|image|archive|plugin|testing|main)(/|\.|$)')
def pkg(fn):
    fn=re.sub(r'\[.*','',fn)
    i=fn.rfind('/'); j=fn.find('.',i+1)
    return fn[:j] if j>0 else fn
def group(p):
    parts=p.split('/')
    if parts[:3]==['github.com','mycroblog','my']: return 'my/'+'/'.join(parts[3:6])
    if parts[0]=='github.com': return '/'.join(parts[:3])
    if '.' in parts[0]: return '/'.join(parts[:2])
    return 'std:'+parts[0]
lib=collections.Counter(); who=collections.Counter(); total=0
def unit(v):
    m=re.match(r'([\d.]+)(B|kB|MB|GB)',v); return float(m[1])*{'B':1,'kB':1024,'MB':1048576,'GB':1<<30}[m[2]]
cur=None; stack=[]
def flush():
    global total
    if cur is None or not stack: return
    if any(f.startswith('runtime/pprof.') for f in stack): return   # the measurement itself
    total+=cur
    l=next((group(pkg(f)) for f in stack if not STD.match(pkg(f))), 'std:'+pkg(stack[0]))
    lib[l]+=cur
    w=None
    for i,f in enumerate(reversed(stack)):
        p=pkg(f)
        if re.search(r'\.init(\.\d+)?(\.func\d+)?$|\.map\.init\.\d+$|\.init$',f) and not f.startswith('runtime.'): w='init of '+group(p); break
        if f.endswith('platform.NewApp'):
            nxt=stack[len(stack)-i-2] if len(stack)-i-2>=0 else f
            w='NewApp: '+re.sub(r'^github.com/mycroblog/my/','my/',re.sub(r'\[.*','',nxt)); break
        if f.endswith('platform.Serve'): w='Serve'; break
    if w is None:
        root=stack[-1]
        w='goroutine: '+re.sub(r'^github.com/mycroblog/my/','my/',re.sub(r'\[.*','',stack[-2] if root.startswith('runtime.goexit') and len(stack)>1 else root))
    who[w]+=cur
for line in sys.stdin:
    line=line.rstrip('\n')
    if line.startswith('-----------'):
        flush(); cur=None; stack=[]; continue
    m=re.match(r'^\s*([\d.]+(?:B|kB|MB|GB))\s+(\S.*)$',line)
    if m and cur is None: cur=unit(m[1]); stack=[m[2].strip()]; continue
    if cur is not None and line.strip() and not line.startswith(('File:','Type:','Time:','Build')): stack.append(line.strip())
flush()
N=int(sys.argv[1]) if len(sys.argv)>1 else 14
print('total (without the profiler): %.2f MB'%(total/1048576))
print('-- by library (the innermost frame outside the standard library)')
for k,v in lib.most_common(N): print('  %6.2f MB %5.1f%%  %s'%(v/1048576,100*v/total,k))
print('-- by who asked')
for k,v in who.most_common(N): print('  %6.2f MB %5.1f%%  %s'%(v/1048576,100*v/total,k))
