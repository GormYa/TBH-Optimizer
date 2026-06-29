#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
extract.py — Extrai os dados de baús do TaskbarHero direto dos arquivos do jogo
(Unity IL2CPP) e gera o mesmo chest_drops.json (formato ChestDoc) + sprites que o
otimizador consome. Substitui o scraping frágil das wikis.

Fontes (dentro de <jogo>/TaskBarHero_Data):
  sharedassets0.assets  -> CSVs *InfoData (TextAsset) + sprites (Sprite)
  StreamingAssets/aa/StandaloneWindows64/localization-*  -> nomes pt-BR (Unity Localization)

Uso:
  python tools/extract.py [out_dir] [--game "D:/.../TaskbarHero"]
  (out_dir padrão: gamedata)

Requer: UnityPy, Pillow.  pip install UnityPy Pillow
"""
import csv
import glob
import io
import json
import os
import re
import struct
import sys

import UnityPy

# ---------------- config / args ----------------
GAME_DEFAULT = r"D:/SteamLibrary/steamapps/common/TaskbarHero"


def parse_args(argv):
    out_dir = "gamedata"
    game = os.environ.get("TBH_GAME", GAME_DEFAULT)
    i18n_only = False
    rest = []
    i = 0
    while i < len(argv):
        a = argv[i]
        if a == "--game" and i + 1 < len(argv):
            game = argv[i + 1]
            i += 2
            continue
        if a == "--i18n-only":
            i18n_only = True
            i += 1
            continue
        rest.append(a)
        i += 1
    if rest:
        out_dir = rest[0]
    return out_dir, game, i18n_only


# ---------------- localização (Unity Localization, sem typetree) ----------------
# Cada entrada serializada é [id u64][len i32][utf8 bytes][padding até múltiplo de 4],
# tanto na SharedTableData (id->NameKey) quanto na StringTable (id->texto localizado).
def _parse_loc_table(raw):
    """Varre o buffer coletando entradas [id u64][len i32][utf8][align4].

    As entradas têm metadata variável entre si, então não dá pra iterar estrito:
    em qualquer ponto que não casar, anda 4 bytes e tenta de novo. Entradas falsas
    (id->lixo) são inofensivas porque o join só usa NameKeys presentes nas duas tabelas.
    """
    out = {}
    n = len(raw)
    i = 0
    while i + 12 <= n:
        ln = struct.unpack_from("<i", raw, i + 8)[0]
        if 1 <= ln <= 400 and i + 12 + ln <= n:
            try:
                s = raw[i + 12:i + 12 + ln].decode("utf-8")
                # exige texto "imprimível": rejeita bytes de controle (lixo) que
                # dessincronizariam o passo e fariam pular entradas reais.
                if s.isprintable():
                    idv = struct.unpack_from("<q", raw, i)[0]
                    out[idv] = s
                    i += 12 + ln
                    i += (-i) % 4
                    continue
            except UnicodeDecodeError:
                pass
        i += 4
    return out


def _mb_raw(bundle_path, want_name):
    env = UnityPy.load(bundle_path)
    for o in env.objects:
        if o.type.name == "MonoBehaviour":
            d = o.read()
            if getattr(d, "m_Name", "") == want_name:
                return o.get_raw_data()
    return None


def load_names(loc_dir, collection, lang="portuguese(brazil)(pt-br)"):
    """Resolve key -> texto localizado para uma coleção (ex.: 'ItemTable' p/ itens,
    'StringTable' p/ UI e nomes de heróis). Cruza SharedTableData (id->key) com a
    StringTable do idioma (id->texto)."""
    shared = os.path.join(loc_dir, "localization-assets-shared_assets_all.bundle")
    # glob: alguns bundles têm sufixo de hash no nome (ex.: vietnamita)
    matches = glob.glob(os.path.join(loc_dir, f"localization-string-tables-{lang}_assets_all*.bundle"))
    if not (os.path.exists(shared) and matches):
        return {}
    table = matches[0]
    id2key = _parse_loc_table(_mb_raw(shared, f"{collection} Shared Data") or b"")
    id2txt = _parse_loc_table(_mb_raw(table, f"{collection}_{key_suffix(lang)}") or b"")
    key2id = {v: k for k, v in id2key.items()}
    return {key: id2txt[idv] for key, idv in key2id.items() if idv in id2txt}


def key_suffix(lang):
    # "portuguese(brazil)(pt-br)" -> "pt-BR"; "chinese(simplified)(zh-hans)" -> "zh-Hans"
    code = lang.rsplit("(", 1)[-1].rstrip(")")
    a, _, b = code.partition("-")
    if not b:
        return a
    # região de 2-3 letras é MAIÚSCULA (pt-BR, en-US); script de 4 é Title (zh-Hans/zh-Hant)
    return f"{a}-{b.upper()}" if len(b) <= 3 else f"{a}-{b.title()}"


def available_langs(loc_dir):
    """Descobre os idiomas presentes nos bundles do jogo. Devolve [(lang, caminho)],
    onde lang é o token do nome do arquivo (ex.: 'portuguese(brazil)(pt-br)').
    Alguns bundles têm sufixo de hash (ex.: vietnamita) -> glob, não nome fixo."""
    out = []
    for p in sorted(glob.glob(os.path.join(loc_dir, "localization-string-tables-*_assets_all*.bundle"))):
        m = re.match(r"localization-string-tables-(.+?)_assets_all", os.path.basename(p))
        if m:
            out.append((m.group(1), p))
    return out


# ---------------- CSVs + sprites do sharedassets0 ----------------
def load_assets(data_dir):
    env = UnityPy.load(os.path.join(data_dir, "sharedassets0.assets"))
    tables = {}
    sprites = {}
    for o in env.objects:
        if o.type.name == "TextAsset":
            d = o.read()
            sc = d.m_Script
            raw = sc.encode("utf-8", "surrogateescape") if isinstance(sc, str) else bytes(sc)
            tables[getattr(d, "m_Name", "")] = raw.decode("utf-8-sig", "replace")
        elif o.type.name == "Sprite":
            d = o.read()
            sprites[getattr(d, "m_Name", "")] = o
    return tables, sprites


def rows(tables, name):
    return list(csv.DictReader(io.StringIO(tables[name])))


# ---------------- regras de domínio ----------------
GRADE_TO_TYPE = {"COMMON": "Common", "RARE": "Stage Boss", "LEGENDARY": "Act Boss"}
# pool base = sem condição de herói (DLC). '501'=Hunter, '601'=Slayer.
BASE_HERO_CONDS = {"", "0"}


def grade_to_type(g):
    return GRADE_TO_TYPE.get(g, g.title() if g else "Common")


# Os baús não são localizados no jogo (NameKey vem em inglês literal). Como os nomes
# seguem padrões fixos, traduzimos pra pt-BR aqui (ex.: "Act Boss Box Lv20" -> "Caixa de Chefe de Ato Nv20").
_BOX_PREFIX = [
    ("Normal Monster Box", "Caixa de Monstro"),
    ("Stage Boss Box", "Caixa de Chefe de Fase"),
    ("Act Boss Box", "Caixa de Chefe de Ato"),
]


def translate_box_name(en):
    for pre, pt in _BOX_PREFIX:
        if en.startswith(pre):
            suffix = en[len(pre):].strip().replace("Lv", "Nv")
            return f"{pt} {suffix}".strip()
    return en


def sprite_rel(icon_path):
    return f"sprites/{icon_path}.png" if icon_path else ""


# Pedras da Alma (190001-4) não são localizadas no jogo (ItemName_19000x sem entrada
# nas tabelas); nomeamos por dificuldade. PT pro padrão atual, EN pros demais idiomas.
SOULSTONE_PT = {"190001": "Pedra da Alma (Normal)", "190002": "Pedra da Alma (Pesadelo)",
                "190003": "Pedra da Alma (Inferno)", "190004": "Pedra da Alma (Tormento)"}
SOULSTONE_EN = {"190001": "Soul Stone (Normal)", "190002": "Soul Stone (Nightmare)",
                "190003": "Soul Stone (Hell)", "190004": "Soul Stone (Torment)"}


# Categoria funcional do material pela faixa do ItemKey (confirmada pelas descrições
# do jogo): 11=decoração, 12=gravação, 13=inscrição, 14=criação, 16=evento, 19=invocação.
def material_category(item_key):
    pre = str(item_key)[:2]
    return {"11": "decoration", "12": "engraving", "13": "inscription",
            "14": "craft", "16": "event", "19": "summon"}.get(pre, "craft")


# ---------------- packs de nomes multilíngues ----------------
def build_name_packs(tables, loc_dir, i18n_dir):
    """Gera site/i18n/names_<código>.json pra CADA idioma do jogo: mapas id->nome
    localizados (itens, baús, fases, monstros, heróis, runas, pets, mods únicos).
    O painel usa esses packs pra trocar os nomes sem reembarcar nada no exe — eles
    são publicados no CDN junto da landing e baixados sob demanda."""
    shared = os.path.join(loc_dir, "localization-assets-shared_assets_all.bundle")
    collections = ("ItemTable", "StringTable")
    key2id = {}
    for c in collections:
        id2key = _parse_loc_table(_mb_raw(shared, f"{c} Shared Data") or b"")
        key2id[c] = {v: k for k, v in id2key.items()}

    items_csv = rows(tables, "ItemInfoData")
    stages_csv = rows(tables, "StageInfoData")
    monsters_csv = rows(tables, "MonsterInfoData")
    heroes_csv = rows(tables, "HeroInfoData")
    runes_csv = rows(tables, "RuneInfoData")
    pets_csv = rows(tables, "PetInfoData")
    umod_ident = {r["UniqueModKey"].strip(): r["UniqueMod"].strip()
                  for r in rows(tables, "UniqueModInfoData")}
    gear_umod = {r["GearKey"].strip(): r.get("UniqueModKey", "").strip()
                 for r in rows(tables, "GearInfoData")}
    stat_mods = {(m["stat"], m["mod"]) for _, mods in material_effects(tables).values() for m in mods}

    os.makedirs(i18n_dir, exist_ok=True)
    written = []
    for lang, table_path in available_langs(loc_dir):
        code = key_suffix(lang)
        id2txt = {}
        for c in collections:
            id2txt[c] = _parse_loc_table(_mb_raw(table_path, f"{c}_{code}") or b"")

        def name(c, nk):
            if not nk:
                return None
            idv = key2id[c].get(nk)
            return id2txt[c].get(idv) if idv is not None else None

        is_pt = code == "pt-BR"
        pack = {"items": {}, "chests": {}, "stages": {}, "monsters": {},
                "heroes": {}, "runes": {}, "pets": {}, "petDesc": {}, "unique": {},
                "itemDesc": {}}

        for r in items_csv:
            iid, nk = r["ItemKey"].strip(), r["NameKey"].strip()
            if r["ITEMTYPE"].strip() == "STAGEBOX":
                # baús não são localizados no jogo (NameKey é inglês literal)
                pack["chests"][iid] = translate_box_name(nk) if is_pt else nk
                continue
            nm = name("ItemTable", nk)
            if nm:
                pack["items"][iid] = nm
            # função dos materiais (aba de materiais do painel)
            if r["ITEMTYPE"].strip() == "MATERIAL":
                ds = name("ItemTable", r.get("DescriptionKey", "").strip())
                if ds:
                    pack["itemDesc"][iid] = ds
        # fallback só pro que o jogo NÃO localiza (não atropela nome oficial)
        for sid, nm in (SOULSTONE_PT if is_pt else SOULSTONE_EN).items():
            pack["items"].setdefault(sid, nm)

        # rótulos do tooltip de material neste idioma (tipos, atributos, cabeçalhos)
        pack.update(material_label_maps(lambda k: name("StringTable", k), stat_mods))

        for r in stages_csv:
            nm = name("StringTable", r.get("StageNameKey", "").strip())
            if nm:
                pack["stages"][r["StageKey"].strip()] = nm
        for r in monsters_csv:
            nm = name("StringTable", r.get("MonsterNameStringKey", "").strip())
            if nm:
                pack["monsters"][r["MonsterKey"].strip()] = nm
        for r in heroes_csv:
            nm = name("StringTable", r.get("HeroNameKey", "").strip())
            if nm:
                pack["heroes"][r["HeroKey"].strip()] = nm
        for r in runes_csv:
            nm = name("StringTable", r.get("NameKey", "").strip())
            if nm:
                pack["runes"][r["RuneKey"].strip()] = nm
        for r in pets_csv:
            nm = name("StringTable", r.get("NameKey", "").strip())
            if nm:
                pack["pets"][r["PetKey"].strip()] = nm
            ds = name("StringTable", r.get("DescriptionKey", "").strip())
            if ds:
                pack["petDesc"][r["PetKey"].strip()] = ds
        for r in items_csv:
            if r["ITEMTYPE"].strip() != "GEAR":
                continue
            umk = gear_umod.get(r["GearKey"].strip(), "")
            if umk and umk != "0":
                txt = name("StringTable", "UniqueMod_" + umod_ident.get(umk, ""))
                if txt:
                    pack["unique"][r["ItemKey"].strip()] = txt

        dest = os.path.join(i18n_dir, f"names_{code}.json")
        with open(dest, "w", encoding="utf-8") as f:
            json.dump(pack, f, ensure_ascii=False, separators=(",", ":"))
        counts = {k: len(v) for k, v in pack.items() if v}
        written.append((code, os.path.getsize(dest), counts))
    return written


# MATERIALTYPE oficial (MaterialInfoData) -> categoria do painel. Substitui a
# heurística por faixa de id (que errava: 16xxxx é OFERENDA, não "evento" genérico).
MTYPE_CAT = {"DECORATION": "decoration", "ENGRAVING": "engraving", "INSCRIPTION": "inscription",
             "CRAFTING": "craft", "OFFERING": "offering", "SOULSTONE": "summon"}


def material_effects(tables):
    """Efeitos de decoração/gravação/inscrição por material, igual ao tooltip do jogo:
    MaterialInfoData -> StatModGroupInfoData (linhas por GearGroup) -> StatModInfoData
    (faixa de valor por tier). Devolve itemKey -> (mtype, [linhas de efeito])."""
    smg = {}
    for r in rows(tables, "StatModGroupInfoData"):
        smg.setdefault(r["StatModGroupKey"].strip(), []).append(r)
    smi = {}
    for r in rows(tables, "StatModInfoData"):
        smi[(r["StatModKey"].strip(), r["Tier"].strip())] = r
    out = {}
    for r in rows(tables, "MaterialInfoData"):
        iid = r["ItemKey"].strip()
        mtype = MTYPE_CAT.get(r["MATERIALTYPE"].strip(), "craft")
        mods = []
        for g in smg.get(r["StatModGroupKey"].strip(), []):
            smk = g["StatModKey"].strip()
            tmin, tmax = g["MinTier"].strip(), g["MaxTier"].strip()
            lo, hi = smi.get((smk, tmin)), smi.get((smk, tmax))
            if not lo or not hi:
                continue
            mods.append({
                "g": g["GearGroup"].strip(),          # WEAPON / ARMOR / ACCESSORY
                "tier": int(tmax or 0),
                "stat": lo["STATTYPE"].strip(),
                "mod": lo["MODTYPE"].strip(),          # FLAT / PERCENT
                "min": float(lo["MinValue"] or 0),
                "max": float(hi["MaxValue"] or 0),
            })
        out[iid] = (mtype, mods)
    return out


# Rótulos do tooltip de material, direto da StringTable do jogo (por idioma):
# tipo ("Material de Gravação"), nome de cada atributo e cabeçalho de efeito por slot.
MAT_TYPE_KEYS = {
    "decoration": "UI_ItemTooltip_DecorationMaterial", "engraving": "UI_ItemTooltip_EngravingMaterial",
    "inscription": "UI_ItemTooltip_InscriptionMaterial", "craft": "UI_ItemTooltip_CraftingMaterial",
    "offering": "UI_ItemTooltip_OfferingMaterial", "summon": "UI_ItemTooltip_SoulstoneMaterial",
}
SLOT_FX_PATTERNS = (
    ("decoration", ("ItemTooltip_Material_DecorationStat_{g}", "ItemTooltip_Material_DecorationStatList_{g}")),
    ("engraving", ("ItemTooltip_Material_EngravingStatList_{g}", "ItemTooltip_Material_EngravingStat_{g}")),
    ("inscription", ("ItemTooltip_Material_InscriptionStatList_{g}", "ItemTooltip_Material_InscriptionStat_{g}")),
)


def material_label_maps(lookup, stat_mods):
    """Monta {matTypes, stats, slotFx, statFmt} usando uma função chave->texto (de
    qualquer idioma). statFmt é a LINHA pronta do tooltip do jogo por stat+modtype
    (ex.: 'Redução de Recarga +{0}~{1}%'); valores em per-mille quando o formato
    tem '%' (o front divide por 10)."""
    mt = {}
    for cat, k in MAT_TYPE_KEYS.items():
        v = lookup(k)
        if v:
            mt[cat] = v
    st = {}
    for s in sorted({s for s, _ in stat_mods}):
        v = lookup("StatName_" + s) or lookup("ShortStatName_" + s)
        if v:
            st[s] = v
    fmt = {}
    for s, m in sorted(stat_mods):
        v = lookup(f"Stat_{s}_{m}_MinMax")
        if v:
            fmt[f"{s}_{m}"] = v
    fx = {}
    for cat, pats in SLOT_FX_PATTERNS:
        for g, gn in (("WEAPON", "Weapon"), ("ARMOR", "Armor"), ("ACCESSORY", "Accessory")):
            for p in pats:
                v = lookup(p.format(g=gn))
                if v:
                    fx[cat + "_" + g] = v
                    break
    return {"matTypes": mt, "stats": st, "slotFx": fx, "statFmt": fmt}


def material_extras(tables):
    """Usos no Cubo: fabricação (tier da receita -> nível do Cubo que destrava) e
    oferenda (moedas de evento têm sub-receita própria com nível)."""
    craft_cube = {}
    offering_cube = {}
    for r in rows(tables, "CubeSubRecipeInfoData"):
        rt = r["RECIPETYPE"].strip()
        if rt == "CRAFTING":
            craft_cube[r["RecipeTier"].strip()] = int(r["UnlockCubeLevel"] or 0)
        elif rt == "OFFERING" and r.get("Material", "").strip():
            mid = r["Material"].strip().split("_")[0]
            offering_cube[mid] = int(r["UnlockCubeLevel"] or 0)
    # material de fabricação -> em quais receitas da forja entra (tipo + tier)
    craft_uses = {}
    for rc in rows(tables, "CraftingRecipeInfoData"):
        tier = int(rc["RecipeTier"] or 0)
        ctype = CRAFT_TYPE_PT.get(rc["ItemCraftingType"].strip(), rc["ItemCraftingType"].strip())
        for tok in rc["Material"].strip().split():
            mid = tok.partition("_")[0]
            if mid:
                craft_uses.setdefault(mid, set()).add((tier, ctype))
    return craft_cube, offering_cube, craft_uses


def _to_int(v, default=0):
    try:
        return int(float(str(v).strip()))
    except (ValueError, TypeError):
        return default


def _to_float(v, default=0.0):
    try:
        return float(str(v).strip())
    except (ValueError, TypeError):
        return default


def build_synthesis(tables):
    """Tabelas de síntese/cubo pro painel (aba Inventário + cabeçalho do Cubo). Tudo
    derivado dos dados do jogo — nada inventado; degrada gracioso se uma tabela faltar.

      odds        por grau: chance de cair/manter/subir ao fundir (GradeInfoData.*GradeWeight).
      amount      por (tipo de síntese × grau): quantos itens fundem (SynthesisRecipeInfoData.MaterialAmount).
      slots       por grau: slots inerentes + extras de enchant (GradeInfoData).
      baseCubeExp por grau: EXP de cubo que o item rende ao ser fundido (GradeInfoData.BaseCubeExp).
      cubeExp     curva de EXP por nível do Cubo (CubeLevelInfoData.ExpForLevelUp).
      recipes     linhas cruas de SynthesisRecipeInfoData (faixas de nível/tier).
      cubeRecipes receitas do Cubo (CubeRecipeInfoData) — pra uso futuro (unlock via save).

    Tipos de síntese (EItemSynthesisType): Gear, Accessory, Material.
    EGradeType real: COMMON→UNCOMMON→RARE→LEGENDARY→IMMORTAL→ARCANA→BEYOND→CELESTIAL→DIVINE→COSMIC."""

    def opt_rows(name):
        try:
            return rows(tables, name)
        except KeyError:
            print(f"  ! tabela ausente: {name} (pulando)")
            return []

    # CsvHelper costuma gravar enum pelo NOME ("Gear"/"COMMON"), mas se vier como int
    # normalizamos pro nome canônico — o front casa por esses nomes (synthType/grade).
    SYNTH_BY_INT = {"0": "Gear", "1": "Accessory", "2": "Material", "3": "None"}
    GRADE_BY_INT = {"0": "COMMON", "1": "UNCOMMON", "2": "RARE", "3": "LEGENDARY", "4": "IMMORTAL",
                    "5": "ARCANA", "6": "BEYOND", "7": "CELESTIAL", "8": "DIVINE", "9": "COSMIC", "10": "NONE"}
    RECIPE_BY_INT = {"0": "ALCHEMY", "1": "SYNTHESIS", "2": "CRAFTING", "3": "DECORATION", "4": "ENGRAVING",
                     "5": "INSCRIPTION", "6": "OFFERING", "7": "EXTRACTION", "8": "NONE"}
    norm_synth = lambda v: SYNTH_BY_INT.get((v or "").strip(), (v or "").strip())
    norm_grade = lambda v: GRADE_BY_INT.get((v or "").strip(), (v or "").strip())
    norm_recipe = lambda v: RECIPE_BY_INT.get((v or "").strip(), (v or "").strip())

    # GradeInfoData -> odds de síntese + slots de enchant + exp de cubo, por grau
    odds, slots, base_cube_exp = {}, {}, {}
    for r in opt_rows("GradeInfoData"):
        g = norm_grade(r.get("GRADE"))
        if not g:
            continue
        w = [_to_int(r.get(c)) for c in ("Lower2GradeWeight", "Lower1GradeWeight",
                                         "SameGradeWeight", "Higher1GradeWeight", "Higher2GradeWeight")]
        tot = sum(w)
        if tot > 0:
            odds[g] = {"down": round((w[0] + w[1]) / tot, 4),
                       "same": round(w[2] / tot, 4),
                       "up": round((w[3] + w[4]) / tot, 4),
                       "weights": w}
        slots[g] = {"inherent": _to_int(r.get("InherentSlotAmount")),
                    "decoration": _to_int(r.get("ExtraSlotAmount_Decoration")),
                    "engraving": _to_int(r.get("ExtraSlotAmount_Engraving")),
                    "inscription": _to_int(r.get("ExtraSlotAmount_Inscription"))}
        base_cube_exp[g] = _to_int(r.get("BaseCubeExp"))

    # SynthesisRecipeInfoData -> quantos itens fundem por (tipo × grau). MaterialAmount
    # pode variar por RecipeTier; pro flag do inventário usamos o menor (limiar mais fácil).
    amount, recipes = {}, []
    for r in opt_rows("SynthesisRecipeInfoData"):
        typ = norm_synth(r.get("ItemSynthesisType"))
        g = norm_grade(r.get("GRADE"))
        amt = _to_int(r.get("MaterialAmount"))
        recipes.append({"key": _to_int(r.get("SynthesisRecipeKey")), "type": typ, "grade": g,
                        "tier": _to_int(r.get("RecipeTier")), "amount": amt,
                        "minMaterialTier": _to_int(r.get("MinMaterialTier")),
                        "minResultLevel": _to_int(r.get("MinResultLevel")),
                        "maxResultLevel": _to_int(r.get("MaxResultLevel"))})
        if typ and g and amt > 0:
            cur = amount.setdefault(typ, {})
            cur[g] = amt if g not in cur else min(cur[g], amt)

    # CubeLevelInfoData -> curva de EXP por nível do Cubo
    cube_exp = {}
    for r in opt_rows("CubeLevelInfoData"):
        cube_exp[str(_to_int(r.get("Level")))] = _to_float(r.get("ExpForLevelUp"))

    # CubeRecipeInfoData -> receitas do Cubo (uso futuro: casar com cubeRecipeSaveDatas do save)
    cube_recipes = []
    for r in opt_rows("CubeRecipeInfoData"):
        cube_recipes.append({"cubeKey": _to_int(r.get("CubeKey")),
                             "recipeType": norm_recipe(r.get("RecipeType") or r.get("RECIPETYPE")),
                             "index": _to_int(r.get("Index")),
                             "defaultUnlocked": (r.get("IsDefaultUnlocked") or "").strip().lower() == "true",
                             "tooltipKey": (r.get("TooltipStringKey") or "").strip()})

    return {"odds": odds, "amount": amount, "slots": slots, "baseCubeExp": base_cube_exp,
            "cubeExp": cube_exp, "recipes": recipes, "cubeRecipes": cube_recipes}


def build(tables, item_names, string_names):
    items = rows(tables, "ItemInfoData")
    groups_raw = rows(tables, "ItemGroupInfoData")
    drops = rows(tables, "DropInfoData")
    stages = rows(tables, "StageInfoData")
    heroes_csv = rows(tables, "HeroInfoData")
    mat_fx = material_effects(tables)
    craft_cube, offering_cube, craft_uses = material_extras(tables)
    grade_gold = {r["GRADE"].strip(): float(r["BaseAlchemyGold"] or 0) for r in rows(tables, "GradeInfoData")}
    # MATERIAL tem escala própria de alquimia (12x; ItemTypeScaleInfoData) -> preço do tooltip
    mat_scale = 1.0
    for r in rows(tables, "ItemTypeScaleInfoData"):
        if r["ItemType"].strip() == "MATERIAL":
            mat_scale = float(r["AlchemyGoldScale"] or 1000) / 1000.0

    # HeroKey -> {nome localizado, dlc}. Nomes de herói ficam na StringTable geral.
    hero_info = {}
    for r in heroes_csv:
        hk = r["HeroKey"].strip()
        nm = string_names.get(r["HeroNameKey"].strip()) or r["ClassType"].strip()
        hero_info[hk] = {"name": nm, "dlc": r["HasDLCDrop"].strip().lower() == "true"}

    # id do item -> registro (nome localizado, grade, icone, tipo)
    item_by_id = {}
    for r in items:
        iid = int(r["ItemKey"])
        nk = r["NameKey"].strip()
        nm = item_names.get(nk, nk)  # gear: ItemName_x->nome; box: NameKey literal (jogo não localiza)
        item_by_id[iid] = {
            "name": nm,
            "grade": r["GRADE"].strip(),
            "icon": r["IconPath"].strip(),
            "type": r["ITEMTYPE"].strip(),
            "dropKey": r["DropKey"].strip(),
            "descKey": r.get("DescriptionKey", "").strip(),
            "tradable": r.get("IsCanExchangeMarketable", "").strip() == "True",
        }

    # ItemGroupKey -> [item ids]
    group_items = {}
    for r in groups_raw:
        group_items.setdefault(int(r["ItemGroupKey"]), []).append(int(r["ItemKey"]))

    # DropKey -> linhas de drop
    drop_by_key = {}
    for r in drops:
        drop_by_key.setdefault(r["DropKey"], []).append(r)

    heroes_used = set()  # heróis (não-base) que aparecem em algum loot -> vira toggle no front

    def loot_for(drop_key):
        # Guarda TODOS os grupos com seu peso e condição de herói. A pool base é
        # HeroKeyCondition vazio/'0'; cada outro herói (DLC ou não) é uma variante que
        # o usuário liga/desliga. A % é recalculada no front conforme a seleção.
        out = []
        for r in drop_by_key.get(drop_key, []):
            if r["REWARDTYPE"] == "ITEMGROUP":
                ids = group_items.get(int(r["RewardKey"]), [])
            elif r["REWARDTYPE"] == "ITEM":
                ids = [int(r["RewardKey"])]  # drop direto (ex.: Pedra da Alma) — grupo de 1
            else:
                continue
            if not ids:
                continue
            cond = r["HeroKeyCondition"].strip()
            hero = "" if cond in BASE_HERO_CONDS else cond
            if hero:
                heroes_used.add(hero)
            out.append({
                "weight": int(r["Weight"]),
                "hero": hero,  # "" = base (sempre conta)
                "grade": item_by_id.get(ids[0], {}).get("grade", ""),
                "items": ids,
            })
        # base primeiro (peso desc), depois por herói
        out.sort(key=lambda g: (g["hero"] != "", g["hero"], -g["weight"]))
        return out

    # box id -> fases onde dropa (via monster/boss + taxa). Rate é décimo de % (160 -> 16.0).
    box_stages = {}
    for r in stages:
        sk = int(r["StageKey"])
        meta = {
            "key": sk,
            "label": f'{r["Act"]}-{r["StageNo"]}',
            "difficulty": r["STAGEDIFFICULITY"].strip(),
        }
        for box_col, rate_col, via in (
            ("MonsterDropItemKey", "MonsterDropItemRate", "monster"),
            ("BossDropItemKey", "BossDropItemRate", "boss"),
        ):
            box = r.get(box_col, "").strip()
            if not box or box == "0":
                continue
            raw_rate = r.get(rate_col, "").strip()
            if raw_rate == "" and via == "boss":
                rate = 100.0  # chefe de ato/boss: rate vazio = drop garantido (100%)
            else:
                try:
                    rate = int(raw_rate or 0) / 10.0
                except ValueError:
                    rate = 0.0
            box_stages.setdefault(int(box), []).append(
                {**meta, "via": via, "dropRatePercent": rate})

    # monta os baús (itens STAGEBOX) + coleta itens referenciados
    chests = []
    referenced = set()
    for iid, it in sorted(item_by_id.items()):
        if it["type"] != "STAGEBOX":
            continue
        loot = loot_for(it["dropKey"])
        st = box_stages.get(iid, [])
        if not loot and not st:
            continue
        for g in loot:
            referenced.update(g["items"])
        chests.append({
            "id": iid,
            "name": translate_box_name(it["name"]),
            "grade": it["grade"],
            "type": grade_to_type(it["grade"]),
            "iconUrl": sprite_rel(it["icon"]),
            "stages": st,
            "loot": loot,
        })

    items_by_id = {}
    for iid in sorted(referenced):
        it = item_by_id.get(iid)
        if not it:
            continue
        cat = "material" if it["type"] == "MATERIAL" else "equip"
        items_by_id[str(iid)] = {"name": it["name"], "grade": it["grade"], "icon": sprite_rel(it["icon"]), "cat": cat}
        if cat == "material":
            e = items_by_id[str(iid)]
            sid = str(iid)
            desc = item_names.get(it["descKey"], "")
            if desc:
                e["desc"] = desc
            mtype, mods = mat_fx.get(sid, (material_category(iid), []))
            e["mcat"] = mtype
            if mods:
                e["mods"] = mods
            e["price"] = round(grade_gold.get(it["grade"], 0) * mat_scale)
            e["tradable"] = it.get("tradable", False)
            if mtype == "craft" and sid in craft_uses:
                e["uses"] = [{"tier": t, "type": ct, "cubeLvl": craft_cube.get(str(t), 0)}
                             for t, ct in sorted(craft_uses[sid])]
            if sid in offering_cube:
                e["cubeLvl"] = offering_cube[sid]

    # heróis que afetam algum loot (vira toggle no front), com nome localizado + flag DLC
    heroes = {hk: hero_info.get(hk, {"name": hk, "dlc": False}) for hk in sorted(heroes_used)}

    # rótulos pt-BR do tooltip de material (baked; outros idiomas vêm do pack i18n)
    stat_mods = {(m["stat"], m["mod"]) for _, mods in mat_fx.values() for m in mods}
    mat_labels = material_label_maps(string_names.get, stat_mods)

    # Pedras das Almas (190001-4): o jogo passou a localizá-las (antes ItemName_19000x
    # não resolvia). A constante agora é só FALLBACK — não atropela o nome oficial.
    for sid, nm in SOULSTONE_PT.items():
        if sid in items_by_id and items_by_id[sid]["name"].startswith("ItemName_"):
            items_by_id[sid]["name"] = nm

    return chests, items_by_id, heroes, mat_labels


# ---------------- sprites ----------------
def export_sprites(chests, items_by_id, sprites, out_dir):
    needed = set()
    for c in chests:
        if c["iconUrl"]:
            needed.add(c["iconUrl"])
    for v in items_by_id.values():
        if v["icon"]:
            needed.add(v["icon"])
    sp_dir = os.path.join(out_dir, "sprites")
    os.makedirs(sp_dir, exist_ok=True)
    ok = miss = 0
    for rel in sorted(needed):
        name = os.path.splitext(os.path.basename(rel))[0]  # "sprites/X.png" -> "X"
        obj = sprites.get(name)
        dest = os.path.join(out_dir, rel.replace("/", os.sep))
        if obj is None:
            miss += 1
            continue
        try:
            img = obj.read().image
            os.makedirs(os.path.dirname(dest), exist_ok=True)
            img.save(dest)
            ok += 1
        except Exception as e:  # noqa: BLE001
            print("  aviso: sprite", name, "-", e)
            miss += 1
    return ok, miss


# ---------------- catalogo completo de equipamentos ----------------
PART_OF = {
    "MAIN_WEAPON": "weapon", "SUB_WEAPON": "offhand",
    "HELMET": "armor", "ARMOR": "armor", "GLOVES": "armor", "BOOTS": "armor",
    "AMULET": "accessory", "EARING": "accessory", "RING": "accessory", "BRACER": "accessory",
}
GEARTYPE_PT = {
    "SWORD": "Espada", "BOW": "Arco", "STAFF": "Cajado", "SCEPTER": "Cetro", "CROSSBOW": "Besta",
    "AXE": "Machado", "SHIELD": "Escudo", "ARROW": "Flecha", "ORB": "Orbe", "TOME": "Tomo",
    "BOLT": "Virote", "HATCHET": "Machadinha", "HELMET": "Elmo", "ARMOR": "Armadura",
    "GLOVES": "Luvas", "BOOTS": "Botas", "AMULET": "Amuleto", "EARING": "Brinco",
    "RING": "Anel", "BRACER": "Bracelete",
}


CRAFT_TYPE_PT = {
    "MainWeapon": "Arma Principal", "SubWeapon": "Arma Secundária",
    "Helmet": "Elmo", "Armor": "Armadura", "Gloves": "Luvas", "Boots": "Botas",
    "Amulet": "Amuleto", "Earing": "Brinco", "Ring": "Anel", "Bracer": "Bracelete",
}


def build_crafting(tables, item_names):
    """Mapeia item criável -> receita(s) da forja. Cada receita é tipo+tier e os
    materiais gastos; o resultado é um equipamento aleatório do grupo (DropKey),
    então um item pode aparecer como um dos resultados possíveis de uma receita."""
    items = rows(tables, "ItemInfoData")
    groups = rows(tables, "ItemGroupInfoData")
    drops = rows(tables, "DropInfoData")
    item_by_id = {int(r["ItemKey"]): r for r in items}
    group_items = {}
    for r in groups:
        group_items.setdefault(int(r["ItemGroupKey"]), []).append(int(r["ItemKey"]))
    drop_by_key = {}
    for r in drops:
        drop_by_key.setdefault(r["DropKey"], []).append(r)

    def produced(drop_key):
        out = set()
        for r in drop_by_key.get(drop_key, []):
            if r["REWARDTYPE"] == "ITEMGROUP":
                out |= set(group_items.get(int(r["RewardKey"]), []))
            elif r["REWARDTYPE"] == "ITEM":
                out.add(int(r["RewardKey"]))
        return out

    def mat_name(mid):
        r = item_by_id.get(mid)
        return item_names.get(r["NameKey"].strip(), r["NameKey"].strip()) if r else ("#" + str(mid))

    def mat_icon(mid):
        r = item_by_id.get(mid)
        return sprite_rel(r["IconPath"].strip()) if r else ""

    rev = {}
    mat_ids = set()
    for rc in rows(tables, "CraftingRecipeInfoData"):
        mats = []
        for tok in rc["Material"].strip().split():
            mid_s, _, qty_s = tok.partition("_")
            if not mid_s:
                continue
            mid = int(mid_s)
            mat_ids.add(mid)
            mats.append({"name": mat_name(mid), "icon": mat_icon(mid), "qty": int(qty_s or 1)})
        recipe = {
            "tier": int(rc["RecipeTier"] or 0),
            "type": CRAFT_TYPE_PT.get(rc["ItemCraftingType"].strip(), rc["ItemCraftingType"].strip()),
            "materials": mats,
        }
        for iid in produced(rc["DropKey"]):
            rev.setdefault(str(iid), []).append(recipe)
    return rev, mat_ids


def _level_scale_fn(tables):
    """Devolve (alchemyScale, cubeScale) por nível, interpolando entre os pontos
    da ItemLevelScaleInfoData (1,5,10,...,90)."""
    pts = []
    for r in rows(tables, "ItemLevelScaleInfoData"):
        pts.append((int(r["Level"]), float(r["AlchemyGoldScale"]), float(r["CubeExpScale"])))
    pts.sort()

    def at(level):
        if level <= pts[0][0]:
            return pts[0][1], pts[0][2]
        if level >= pts[-1][0]:
            return pts[-1][1], pts[-1][2]
        for i in range(1, len(pts)):
            if level <= pts[i][0]:
                l0, a0, c0 = pts[i - 1]
                l1, a1, c1 = pts[i]
                t = (level - l0) / (l1 - l0)
                return a0 + (a1 - a0) * t, c0 + (c1 - c0) * t
        return pts[-1][1], pts[-1][2]
    return at


def build_items_catalog(tables, item_names, sprites, out_dir, drop_ids, crafting=None, mat_ids=None, string_names=None):
    """Catalogo COMPLETO de equipamentos (todos os tiers, inclusive síntese-only) p/
    a aba Itens: tipo, nível, raridade, slots, ouro de alquimia, exp do cubo,
    se dropa e se ainda é obtível."""
    # base por grade (alquimia/cubo/slots), confirmado contra o jogo:
    #   alquimia = BaseAlchemyGold x (alchemyLevelScale/1000); cubo idem com CubeExp.
    grade = {}
    for r in rows(tables, "GradeInfoData"):
        grade[r["GRADE"].strip()] = {
            "ag": float(r["BaseAlchemyGold"] or 0),
            "ce": float(r["BaseCubeExp"] or 0),
            "slotInherent": int(r["InherentSlotAmount"] or 0),
            "slotDec": int(r["ExtraSlotAmount_Decoration"] or 0),
            "slotEng": int(r["ExtraSlotAmount_Engraving"] or 0),
            "slotIns": int(r["ExtraSlotAmount_Inscription"] or 0),
        }
    lvl_scale = _level_scale_fn(tables)
    # stats base/inerentes por item: ItemInfoData.GearKey -> GearInfoData (valores)
    # + GearTypeInfoData (quais STATTYPEs sao os base de cada tipo).
    gear_base_types = {}
    for r in rows(tables, "GearTypeInfoData"):
        gear_base_types[r["GearType"].strip()] = (
            r["BaseStat1_STATTYPE"].strip(), r["BaseStat1_MODTYPE"].strip(),
            r["BaseStat2_STATTYPE"].strip(), r["BaseStat2_MODTYPE"].strip())
    gear_info = {r["GearKey"].strip(): r for r in rows(tables, "GearInfoData")}
    # mod único (skill especial) dos equipamentos arcano+: UniqueModKey -> identificador
    # (UniqueModInfoData) -> texto localizado "UniqueMod_<id>" na StringTable.
    sn = string_names or {}
    umod_ident = {r["UniqueModKey"].strip(): r["UniqueMod"].strip() for r in rows(tables, "UniqueModInfoData")}
    umod_desc = {umk: sn.get("UniqueMod_" + ident, "") for umk, ident in umod_ident.items()}
    items = {}
    for r in rows(tables, "ItemInfoData"):
        if r["ITEMTYPE"] != "GEAR":
            continue
        iid = r["ItemKey"].strip()
        g = r["GRADE"].strip()
        lv = int(r["Level"] or 0)
        gi = grade.get(g, {})
        a_sc, c_sc = lvl_scale(lv) if lv else (1000.0, 1000.0)
        gd = gear_info.get(r["GearKey"].strip(), {})
        bt1, bm1, bt2, bm2 = gear_base_types.get(r["GEARTYPE"].strip(), ("NONE", "FLAT", "NONE", "FLAT"))
        base = []
        for st, mod, val in ((bt1, bm1, gd.get("BaseStat1_Value")), (bt2, bm2, gd.get("BaseStat2_Value"))):
            if st and st != "NONE" and val and val not in ("0",):
                base.append({"stat": st, "mod": mod, "value": float(val)})
        inh = []
        for i in (1, 2, 3):
            st = gd.get("InherentStat%d_STATTYPE" % i, "NONE")
            if st and st != "NONE":
                inh.append({"stat": st, "mod": gd.get("InherentStat%d_MODTYPE" % i, "FLAT").strip(),
                            "value": float(gd.get("InherentStat%d_Value" % i, 0) or 0)})
        atk = None
        if bt1 == "AttackDamage" and gd.get("BaseStat1_Value"):
            atk = round(float(gd["BaseStat1_Value"]))
        items[iid] = {
            "name": item_names.get(r["NameKey"].strip(), r["NameKey"].strip()),
            "grade": g,
            "part": PART_OF.get(r["PARTS"].strip(), ""),
            "gear": GEARTYPE_PT.get(r["GEARTYPE"].strip(), r["GEARTYPE"].strip().title()),
            "level": lv,
            "icon": sprite_rel(r["IconPath"].strip()),
            "obtainable": r["IsDeletedInServer"].strip() != "True",
            "drops": int(iid) in drop_ids,
            "alchemyGold": round(gi.get("ag", 0) * a_sc / 1000.0),
            "cubeExp": round(gi.get("ce", 0) * c_sc / 1000.0),
            "slots": {"inh": gi.get("slotInherent", 0), "dec": gi.get("slotDec", 0), "eng": gi.get("slotEng", 0), "ins": gi.get("slotIns", 0)},
            "tradable": r["IsCanExchangeMarketable"].strip() == "True",
            "atk": atk,
            "base": base,
            "inherent": inh,
        }
        rc = (crafting or {}).get(iid)
        if rc:
            items[iid]["crafting"] = rc
        umk = gd.get("UniqueModKey", "").strip()
        if umk and umk != "0":
            desc = umod_desc.get(umk, "")
            if desc:
                items[iid]["unique"] = desc
    # exporta os ícones que ainda não vieram do loot (so 396 únicos no total)
    needed = {it["icon"] for it in items.values() if it["icon"]}
    # + ícones dos materiais de criação (podem não estar no loot dos baús)
    if mat_ids:
        icon_by_id = {r["ItemKey"].strip(): r["IconPath"].strip() for r in rows(tables, "ItemInfoData")}
        for mid in mat_ids:
            ic = icon_by_id.get(str(mid), "")
            if ic:
                needed.add(sprite_rel(ic))
    ok = 0
    for rel in needed:
        dest = os.path.join(out_dir, rel.replace("/", os.sep))
        if os.path.exists(dest):
            ok += 1
            continue
        obj = sprites.get(os.path.splitext(os.path.basename(rel))[0])
        if obj is None:
            continue
        try:
            os.makedirs(os.path.dirname(dest), exist_ok=True)
            obj.read().image.save(dest)
            ok += 1
        except Exception:
            pass
    dest = os.path.join(out_dir, "items.json")
    with open(dest, "w", encoding="utf-8") as f:
        json.dump({"items": items}, f, ensure_ascii=False)
    obt = sum(1 for v in items.values() if v["obtainable"])
    print(f"Catálogo de equipamentos: {len(items)} ({obt} obtíveis) · ícones {ok} -> {dest}")


# ---------------- main ----------------
def main():
    out_dir, game, i18n_only = parse_args(sys.argv[1:])
    data_dir = os.path.join(game, "TaskBarHero_Data")
    loc_dir = os.path.join(data_dir, "StreamingAssets", "aa", "StandaloneWindows64")
    if not os.path.isdir(data_dir):
        print("ERRO: pasta do jogo não encontrada:", data_dir)
        sys.exit(1)

    version = "?"
    vpath = os.path.join(game, "Version.txt")
    if os.path.exists(vpath):
        version = open(vpath, encoding="utf-8", errors="replace").read().strip()

    print("Lendo assets do jogo… (v%s)" % version)
    tables, sprites = load_assets(data_dir)

    # packs de nomes multilíngues -> site/i18n (publicados no CDN, não embarcados)
    repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    i18n_dir = os.path.join(repo_root, "site", "i18n")
    print("Gerando packs de nomes multilíngues…")
    for code, size, counts in build_name_packs(tables, loc_dir, i18n_dir):
        print(f"  {code}: {size//1024} KB · " + ", ".join(f"{k}={v}" for k, v in counts.items()))
    if i18n_only:
        print("OK (--i18n-only)")
        return

    print("Resolvendo nomes pt-BR…")
    item_names = load_names(loc_dir, "ItemTable")
    string_names = load_names(loc_dir, "StringTable")
    print("  nomes de item:", len(item_names), "| strings:", len(string_names))

    chests, items_by_id, heroes, mat_labels = build(tables, item_names, string_names)
    print(f"Baús: {len(chests)} · itens: {len(items_by_id)} · heróis no loot: "
          + ", ".join(f"{k}={v['name']}" for k, v in heroes.items()))

    print("Exportando sprites…")
    ok, miss = export_sprites(chests, items_by_id, sprites, out_dir)
    print(f"  sprites: {ok} ok, {miss} faltando")

    doc = {
        "_schema": {
            "source": f"arquivos do jogo TaskbarHero v{version} (extração local UnityPy)",
            "dropRate": "stages[].dropRatePercent em %; rate do CSV /10",
            "loot": "loot[].weight + hero ('' = base). % do grupo = weight / soma dos pesos dos grupos ativos (base + heróis selecionados). por item ~ %/len(items)",
            "heroes": "id -> {name, dlc}; heróis cuja posse muda a pool de loot",
        },
        "updatedAt": "",  # preenchido pelo CI/Go
        "gameVersion": version,
        "heroes": heroes,
        "chests": chests,
        "itemsById": items_by_id,
        "matLabels": mat_labels,
    }
    os.makedirs(out_dir, exist_ok=True)
    dest = os.path.join(out_dir, "chest_drops.json")
    with open(dest, "w", encoding="utf-8") as f:
        json.dump(doc, f, ensure_ascii=False, indent=1)
    print("OK ->", dest)

    drop_ids = {int(k) for k in items_by_id}
    crafting, mat_ids = build_crafting(tables, item_names)
    print(f"Receitas de criação: {len(crafting)} itens criáveis · {len(mat_ids)} materiais")
    build_items_catalog(tables, item_names, sprites, out_dir, drop_ids, crafting, mat_ids, string_names)

    # síntese + curva do Cubo (Etapa 3): odds reais de fundir, qtd por grau, EXP por nível
    print("Síntese / Cubo…")
    synth = build_synthesis(tables)
    synth["_schema"] = {
        "odds": "por grau: down/same/up (fração) ao fundir + weights crus [L2,L1,Same,H1,H2] (GradeInfoData)",
        "amount": "tipo de síntese (Gear/Accessory) -> grau -> nº de itens p/ fundir (SynthesisRecipeInfoData.MaterialAmount)",
        "cubeExp": "nível do Cubo -> EXP p/ subir de nível (CubeLevelInfoData.ExpForLevelUp)",
        "slots/baseCubeExp": "por grau: slots de enchant e EXP que o item rende (GradeInfoData)",
    }
    synth["gameVersion"] = version
    dest_s = os.path.join(out_dir, "synthesis.json")
    with open(dest_s, "w", encoding="utf-8") as f:
        json.dump(synth, f, ensure_ascii=False, indent=1)
    print("OK ->", dest_s)
    # resumo pra conferência (os números reais ficam visíveis aqui)
    for g, o in synth["odds"].items():
        print(f"    {g:10s} ↓{o['down'] * 100:5.1f}%  ={o['same'] * 100:5.1f}%  ↑{o['up'] * 100:5.1f}%   pesos {o['weights']}")
    print("  qtd p/ fundir:", {t: dict(v) for t, v in synth["amount"].items()})
    cl = synth["cubeExp"]
    print(f"  níveis de Cubo: {len(cl)}  (ex.: nv1={cl.get('1')}, nv2={cl.get('2')}, nv10={cl.get('10')})  ·  receitas: {len(synth['cubeRecipes'])}")


if __name__ == "__main__":
    main()
