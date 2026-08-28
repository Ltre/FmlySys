(() => {
  const escapeHTML = (value) => String(value ?? '').replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
  const formatTime = (value) => {
    if (!value) return 'â€”';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return escapeHTML(value);
    const pad = (n) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
  };
  const requestJSON = async (url, options = {}) => {
    const response = await fetch(url, { credentials: 'same-origin', cache: 'no-store', ...options });
    const type = response.headers.get('content-type') || '';
    const payload = type.includes('application/json') ? await response.json() : null;
    if (!response.ok || (payload && payload.ok === false)) {
      throw new Error((payload && payload.message) || `è¯·æ±‚å¤±è´¥ï¼ˆHTTP ${response.status}ï¼‰`);
    }
    return payload;
  };
  const postForm = async (url, values) => requestJSON(url, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8',
      'X-Fmly-Async': '1'
    },
    body: new URLSearchParams(values)
  });

  function enhanceMemberManagement() {
    const list = document.querySelector('.member-permission-list');
    if (!list) return;
    const section = list.closest('section.card');
    if (section) section.id = 'members-and-permissions';

    list.querySelectorAll('form.member-permission-card').forEach((form) => {
      const match = form.getAttribute('action')?.match(/^\/admin\/members\/(\d+)\/permissions$/);
      if (!match || form.querySelector('[data-member-profile-editor]')) return;
      const memberID = match[1];
      const summary = form.querySelector('strong');
      const relationNode = form.querySelector('.muted');
      const currentName = (summary?.textContent || '').replace(/^#\d+\s*/, '').trim();
      const currentRelation = (relationNode?.textContent || '').trim();

      const details = document.createElement('details');
      details.className = 'wide';
      details.dataset.memberProfileEditor = '1';
      details.innerHTML = `<summary class="secondary-button">ä¿®æ”¹æˆå‘˜ä¿¡æ¯ / æ ‡è®°åˆ é™¤</summary>
        <div class="form inner-form">
          <input data-member-name value="${escapeHTML(currentName)}" placeholder="æˆå‘˜å§“å" maxlength="80" required>
          <input data-member-relation value="${escapeHTML(currentRelation)}" placeholder="å…³ç³»/å¤‡æ³¨" maxlength="160">
          <button type="button" data-member-save>ä¿å­˜æˆå‘˜ä¿¡æ¯</button>
          <button type="button" class="danger-button" data-member-delete>æ ‡è®°åˆ é™¤</button>
          <span class="wide form-feedback" data-member-status hidden></span>
        </div>`;
      const permissionGrid = form.querySelector('.permission-grid');
      if (permissionGrid) permissionGrid.before(details);
      else form.append(details);

      details.querySelectorAll('input').forEach((input) => input.addEventListener('keydown', (event) => {
        if (event.key === 'Enter') {
          event.preventDefault();
          details.querySelector('[data-member-save]').click();
        }
      }));
      const status = details.querySelectoŠ	ÖÙ]K[Y[X™\‹\İ]\×IÊNÂˆÛÛœİÚİÔİ]\ÈH
Y\ÜØYÙK\œ›ÜˆH˜[ÙJHOˆÂˆİ]\ËšY[ˆH[Y\ÜØYÙNÂˆİ]\Ë^ÛÛ[HY\ÜØYÙH	ÉÎÂˆİ]\Ë˜Û\ÜÓ\İÙÙÛJ	Ú\ËY\œ›Ü‰Ë\œ›ÜŠNÂˆNÂˆ]Z[Ëœ]Y\TÙ[XİÜŠ	ÖÙ]K[Y[X™\‹\Ø]™WIÊK˜Y]™[\İ[™\Š	ØÛXÚÉË\Ş[˜È
]™[
HOˆÂˆÛÛœİ]ÛˆH]™[˜İ\œ™[\™Ù]Âˆ]Û‹™\ØX›YHYNÂˆÚİÔİ]\Ê	ù«hùg*9/çykf9¢$9df9/èy kø )‰ÊNÂˆHÂˆ]ØZ]Üİ›Ü›JØYZ[‹ÛY[X™\œËÉÛY[X™\’QKÙY]Âˆ˜[YNˆ]Z[Ëœ]Y\TÙ[XİÜŠ	ÖÙ]K[Y[X™\‹[˜[YWIÊK˜[YKˆ™[][Ûˆ]Z[Ëœ]Y\TÙ[XİÜŠ	ÖÙ]K[Y[X™\‹\™[][Û—IÊK˜[YBˆJNÂˆÚ[™İË›ØØ][Û‹œ™[ØY

NÂˆHØ]Ú
\œ›ÜŠHÂˆÚİÔİ]\Ê\œ›Ü‹›Y\ÜØYÙHİš[™Ê\œ›ÜŠKYJNÂˆ]Û‹™\ØX›YH˜[ÙNÂˆBˆJNÂˆ]Z[Ëœ]Y\TÙ[XİÜŠ	ÖÙ]K[Y[X™\‹Y[]WIÊK˜Y]™[\İ[™\Š	ØÛXÚÉË\Ş[˜È
]™[
HOˆÂˆYˆ
]Ú[™İË˜ÛÛ™š\›J	ùcê¹/&¹¨!ú+¬9b(:fi;ï#9.#y/&¹b(:fi:-a:aäy­`y¬-9¢%¹k¨z+¨z+¬9oexà ¹.áyodú+éy¢$9df9odùbcy£ y§"yak9alz-a9.©ù/fzh§y..ˆ9¥í¹a`z+®9îéùîëxà ¹èkº+©9¨!ú+¬9b(:fi;ï'ÉÊJH™]\›ÂˆÛÛœİ]ÛˆH]™[˜İ\œ™[\™Ù]Âˆ]Û‹™\ØX›YHYNÂˆÚİÔİ]\Ê	ù«hùg*9¨à9§éy/fzh§ynm¹¨!ú+¬9b(:fi8 )‰ÊNÂˆHÂˆ]ØZ]Üİ›Ü›JØYZ[‹ÛY[X™\œËÉÛY[X™\’QKÙ[]XßJNÂˆÚ[™İË›ØØ][Û‹œ™[ØY

NÂˆHØ]Ú
\œ›ÜŠHÂˆÚİÔİ]\Ê\œ›Ü‹›Y\ÜØYÙHİš[™Ê\œ›ÜŠKYJNÂˆ]Û‹™\ØX›YH˜[ÙNÂˆBˆJNÂˆJNÂˆB‚ˆ[˜İ[Ûˆ]šY[˜ÙRS
][\ÊHÂˆYˆ
P\œ˜^Kš\Ğ\œ˜^J][\ÊH][\Ë›[™İOOH
H™]\›ˆ	ø %	ÎÂˆ™]\›ˆ][\Ë›X\

][JHOˆHÛ\ÜÏH˜]XÚY[ˆ\™Ù]H—Ø›[šÈˆ™YH‹Ù]šY[˜ÙKÉÙ[˜ÛÙUT’PÛÛ\Û™[
][KšY
_H‰Ù\ØØ\RS
][K›˜[YJ_OØO˜
Kš›Ú[Š	È	ÊNÂˆB‚ˆ[˜İ[Ûˆ]ZXÚĞXİ[Û’S
›İJHÂˆYˆ
›İKœİ]\ÈOOH	Ù˜Y	ÊHÂˆ™]\›ˆHÛ\ÜÏH˜]Û‹[ZÙHˆ™YH‹ØYZ[‹Ü]ZXÚË[[Û™^K[›İK]Ë\İ[™\š^™YÚYIÙ[˜ÛÙUT’PÛÛ\Û™[
›İKšY
_Hº/æú(c9¥l9£k¹aiyn¤ÏØO˜ÂˆBˆYˆ
›İKœİ[™\™^™Yİ\H	‰ˆ›İKœİ[™\™^™YÚY
HÂˆ™]\›ˆH™YH‹Û[Û™^K\™XÛÜ™ÉÙ[˜ÛÙUT’PÛÛ\Û™[
›İKœİ[™\™^™Yİ\J_KÉÙ[˜ÛÙUT’PÛÛ\Û™[
›İKœİ[™\™^™YÚY
_H¹mì¹aiyn¤È0­È9§éyç"ù«hùo#ù¥l9£kØO˜ÂˆBˆ™]\›ˆ	ùmì¹aiyn¤ÉÎÂˆB‚ˆ\Ş[˜È[˜İ[Ûˆ[š™Xİ]ZXÚÓ[Û™^TÙXİ[ÛŠ
HÂˆÛÛœİY[X™\“\İHØİ[Y[œ]Y\TÙ[XİÜŠ	Ë›Y[X™\‹\\›Z\ÜÚ[Û‹[\İ	ÊNÂˆÛÛœİY[X™\”ÙXİ[ÛˆHY[X™\“\İË˜ÛÜÙ\İ
	ÜÙXİ[Û‹˜Ø\™	ÊNÂˆYˆ
[Y[X™\”ÙXİ[ÛˆØİ[Y[™Ù][[Y[RY
	ØYZ[‹\]ZXÚË[[Û™^K[›İ\ÉÊJH™]\›ÂˆÛÛœİÙXİ[ÛˆHØİ[Y[˜Ü™X]Q[[Y[
	ÜÙXİ[Û‰ÊNÂˆÙXİ[Û‹˜Û\ÜÓ˜[YHH	ØØ\™	ÎÂˆÙXİ[Û‹šYH	ØYZ[‹\]ZXÚË[[Û™^K[›İ\ÉÎÂˆÙXİ[Û‹š[›™\’SH]ˆÛ\ÜÏHœÙXİ[Û‹ZXY]¹bcycì9oêú`'ú+¬9oeOÚÛ\ÜÏH›]]Y¹ë¨yä!¹df9cëùi!9ä!¹¢`9§"y¢$9df9£ä9.©9æ¡9oêú`'ú+¬9oe{ï&ù«hùo#ùaiyn¤ù¥íº-a:aäy..ù/dù.ãy£"yc§ú+¬9oey.®º+¨yë¥ûï#:fa9.í¹d£9¤f:) y¬¯ùå*9c§ú+¬9oexà ÜÙ]Ù]‚ˆX›O’Qİº+¬9oey.®İ¹b!¹ìnÏİ¹¤f:) Oİº+¬9oey¥íºeíİºfa9.íİ¹i!9ä!İİ›ÙH]KXYZ[‹\]ZXÚË[[Û™^KX›ÙOÛÛÜ[HÈˆÛ\ÜÏH›]]Y¹«hùg*9b¨:/ox )İİİ›ÙOİX›O˜ÂˆY[X™\”ÙXİ[Û‹˜Y\ŠÙXİ[ÛŠNÂˆÛÛœİ›ÙHHÙXİ[Û‹œ]Y\TÙ[XİÜŠ	ÖÙ]KXYZ[‹\]ZXÚË[[Û™^KX›ÙWIÊNÂˆHÂˆÛÛœİ^[ØYH]ØZ]™\]Y\İ”ÓÓŠ	ËØYZ[‹Ø\KÜ]ZXÚË[[Û™^K[›İ\ÉËÈXY\œÎˆÈXØÙ\ˆ	Ø\XØ][Û‹ÚœÛÛ‰ÈHJNÂˆÛÛœİ›İ\ÈH^[ØYË››İ\È×NÂˆYˆ
›İ\Ë›[™İOOH
HÂˆ›ÙKš[›™\’SH	ÏÛÛÜ[HÈˆÛ\ÜÏH›]]Y¹¦ ¹¥è9oêú`'ú+¬9oexà İİ‰ÎÂˆ™]\›ÂˆBˆ›ÙKš[›™\’SH›İ\Ë›X\

›İJHOˆ‚ˆˆÉÙ\ØØ\RS
›İKšY
_Oİ‚ˆ‰Ù\ØØ\RS
›İK˜Ü™X]Ü—Û˜[YJ_OœÜ[ˆÛ\ÜÏH›]]Y¹¢$9dfÉÙ\ØØ\RS
›İK˜Ü™X]YØJ_OÜÜ[İ‚ˆ‰Ù\ØØ\RS
›İK˜Ø]YÛÜWÛX™[
_Oİ‚ˆ‰Ù\ØØ\RS
›İKœİ[[X\J_Oİ‚ˆ‰Ù›Ü›X][YJ›İK˜Ü™X]YØ]
_Oİ‚ˆ‰Ù]šY[˜ÙRS
›İK™]šY[˜ÙJ_Oİ‚ˆ‰Ü]ZXÚĞXİ[Û’S
›İJ_Oİ‚ˆİ˜
Kš›Ú[Š	ÉÊNÂˆHØ]Ú
\œ›ÜŠHÂˆ›ÙKš[›™\’SHÛÛÜ[HÈˆÛ\ÜÏH™[™Ù\ˆ¹b¨:/oyoêú`'ú+¬9oeyi,z-){ï&‰Ù\ØØ\RS
\œ›Ü‹›Y\ÜØYÙH\œ›ÜŠ_Oİİ˜ÂˆBˆB‚ˆ\Ş[˜È[˜İ[Ûˆ[š[˜ÙU˜[œÙ™\•X›J
HÂˆÛÛœİX›HHØİ[Y[œ]Y\TÙ[XİÜŠ	Èİ˜[œÙ™\‹\™XÛÜ™ÈX›IÊNÂˆYˆ
]X›HX›K™]\Ù]œ\œÜÙQ[š[˜ÙYOOH	ÌIÊH™]\›ÂˆX›K™]\Ù]œ\œÜÙQ[š[˜ÙYH	ÌIÎÂˆÛÛœİXY\ˆHX›Kœ]Y\TÙ[XİÜŠ	İ‰ÊNÂˆYˆ
ZXY\ŠH™]\›ÂˆÛÛœİHØİ[Y[˜Ü™X]Q[[Y[
	İ	ÊNÂˆ^ÛÛ[H	ùå*:`%ù.¢ùb¨IÎÂˆXY\‹š[œÙ\™Y›Ü™JXY\‹›\İ[[Y[Ú[
NÂ‚ˆÛÛœİ›İÜÈHË‹‹X›Kœ]Y\TÙ[XİÜ[
	İ–Ù]K\™XÛÜ™ZÙ^WH˜[œÙ™\ˆ—IÊWNÂˆ›İÜË™›Ü‘XXÚ

›İÊHOˆÂˆÛÛœİHØİ[Y[˜Ü™X]Q[[Y[
	İ	ÊNÂˆ˜Û\ÜÓ˜[YHH	Û]]Y	ÎÂˆ^ÛÛ[H	ùb¨:/oy.+x )‰ÎÂˆ›İËš[œÙ\™Y›Ü™J›İË›\İ[[Y[Ú[
NÂˆ›İË™]\Ù]œ\œÜÙPÙ[H	ÌIÎÂˆJNÂˆÛÛœİ[\T›İÈHX›Kœ]Y\TÙ[XİÜŠ	İ››İ
Ù]K\™XÛÜ™ZÙ^WJHØÛÛÜ[HH—IÊNÂˆYˆ
[\T›İÊH[\T›İË˜ÛÛÜ[ˆHÂ‚ˆHÂˆÛÛœİ^[ØYH]ØZ]™\]Y\İ”ÓÓŠ	ËØYZ[‹Ø\Kİ˜[œÙ™\œÉËÈXY\œÎˆÈXØÙ\ˆ	Ø\XØ][Û‹ÚœÛÛ‰ÈHJNÂˆÛÛœİRQH™]ÈX\

^[ØYË˜[œÙ™\œÈ×JK›X\

][JHOˆÔİš[™Ê][KšY
K][WJJNÂˆ›İÜË™›Ü‘XXÚ

›İÊHOˆÂˆÛÛœİYH›İË™]\Ù]œ™XÛÜ™Ù^KœÜ]
	Î‰ÊVÌWNÂˆÛÛœİ][HHRQ™Ù]
Y
NÂˆÛÛœİÙ[H›İËœ]Y\TÙ[XİÜŠ	İÙ]K\\œÜÙKXÙ[K›]]Y›\İ[Ù‹]\IÊNÂˆÛÛœİ\™Ù]H›İËœ]Y\TÙ[XİÜŠ	İ›[\İXÚ[
ŠIÊNÂˆÛÛœİ\œÜÙPÙ[H\™Ù]	‰ˆ›İË™]\Ù]œ\œÜÙPÙ[OOH	ÌIÈÈ\™Ù]ˆÙ[ÂˆYˆ
\\œÜÙPÙ[
H™]\›ÂˆYˆ
Z][JHÂˆ\œÜÙPÙ[^ÛÛ[H	ø %	ÎÂˆ™]\›ÂˆBˆÛÛœİ\ÈH×NÂˆYˆ
][Kœ\œÜÙJH\Ëœ\Ú
\ØØ\RS
][Kœ\œÜÙJJNÂˆYˆ
][K›X]\—İ]JH\Ëœ\Ú
Ü[ˆÛ\ÜÏH›]]Y¹.¢ùb¨{ï&‰Ù\ØØ\RS
][K›X]\—İ]J_OÜÜ[˜
NÂˆ\œÜÙPÙ[š[›™\’SH\Ë›[™İÈ\Ëš›Ú[Š	Ïœ‰ÊHˆ	ø %	ÎÂˆJNÂˆHØ]Ú
\œ›ÜŠHÂˆ›İÜË™›Ü‘XXÚ

›İÊHOˆÂˆÛÛœİ\™Ù]H›İËœ]Y\TÙ[XİÜŠ	İ›[\İXÚ[
ŠIÊNÂˆYˆ
\™Ù]
H\™Ù]^ÛÛ[H	ùb¨:/oyi,z-)IÎÂˆJNÂˆBˆB‚ˆ[š[˜ÙSY[X™\“X[˜YÙ[Y[

NÂˆ[š™Xİ]ZXÚÓ[Û™^TÙXİ[ÛŠ
NÂˆ[š[˜ÙU˜[œÙ™\•X›J
NÂŸJJ
NÂ