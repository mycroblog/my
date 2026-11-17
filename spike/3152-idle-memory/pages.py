# pages.py <pid> <binary with symbols> <nm -size output>: the resident pages of the binary's mappings, by section
# and, for the text, by module. Reads /proc/<pid>/pagemap (bit 63: the page is present in this process).
import sys,re,struct,subprocess,collections,bisect
pid,binary,nmfile=sys.argv[1:4]
PAGE=4096
def present(start,end):
    out=[]
    with open(f'/proc/{pid}/pagemap','rb') as f:
        f.seek(start//PAGE*8)
        data=f.read((end-start)//PAGE*8)
    for i in range(len(data)//8):
        if struct.unpack_from('<Q',data,i*8)[0]>>63: out.append(start+i*PAGE)
    return out
pages=set()
for line in open(f'/proc/{pid}/maps'):
    m=re.match(r'([0-9a-f]+)-([0-9a-f]+) (\S+) .*'+re.escape(binary.split('/')[-1])+'$',line.strip())
    if m: pages.update(present(int(m[1],16),int(m[2],16)))
sp=sorted(pages)
def resident(a,size):
    if size<=0: return 0
    lo=bisect.bisect_left(sp,a-a%PAGE); n=0
    for p in sp[lo:]:
        if p>=a+size: break
        n+=min(p+PAGE,a+size)-max(p,a)
    return n
print('-- by section (resident / size, MB)')
for line in subprocess.run(['readelf','-S','-W',binary],capture_output=True,text=True).stdout.splitlines():
    m=re.search(r'\] (\.\S+)\s+(PROGBITS|NOBITS)\s+([0-9a-f]+)\s+[0-9a-f]+\s+([0-9a-f]+)',line)
    if m and int(m[4],16)>1<<20:
        a,s=int(m[3],16),int(m[4],16); print('  %-14s %6.1f / %6.1f'%(m[1],resident(a,s)/1048576,s/1048576))
def group(name):
    n=re.sub(r'\[.*','',name); i=n.rfind('/'); j=n.find('.',i+1); p=(n[:j] if j>0 else n).split('/')
    if p[:3]==['github.com','mycroblog','my']: return 'my/'+'/'.join(p[3:5])
    if p[0]=='github.com': return '/'.join(p[:3])
    if '.' in p[0]: return '/'.join(p[:2])
    return 'std+runtime'
res=collections.Counter(); tot=collections.Counter()
for line in open(nmfile):
    f=line.split(None,3)
    if len(f)<4 or f[2] not in 'Tt': continue
    a,s=int(f[0],16),int(f[1]); g=group(f[3].strip()); tot[g]+=s; res[g]+=resident(a,s)
print('-- text by module (resident / size, MB): %.1f / %.1f'%(sum(res.values())/1048576,sum(tot.values())/1048576))
for g,v in res.most_common(int(sys.argv[4]) if len(sys.argv)>4 else 18): print('  %6.1f / %6.1f  %s'%(v/1048576,tot[g]/1048576,g))
