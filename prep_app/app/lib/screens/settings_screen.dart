import 'dart:async';

import 'package:flutter/material.dart';

import '../api.dart';
import '../model_catalog.dart';
import '../theme.dart';

class SettingsScreen extends StatefulWidget {
  const SettingsScreen({
    super.key,
    required this.api,
    required this.onChangePassword,
    required this.onLogout,
  });

  final Api api;
  final VoidCallback onChangePassword;
  final VoidCallback onLogout;

  @override
  State<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends State<SettingsScreen> {
  final _query = TextEditingController();
  Timer? _debounce;
  ModelSettings? _settings;
  CatalogPage? _page;
  String? _error;
  String? _saveError;
  bool _loadingSettings = true;
  bool _loading = false;
  bool _resetting = false;
  String? _saving;
  int _ticket = 0;

  @override
  void initState() {
    super.initState();
    _loadSettings();
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _query.dispose();
    super.dispose();
  }

  Future<void> _loadSettings() async {
    setState(() {
      _loadingSettings = true;
      _error = null;
    });
    try {
      final settings = await widget.api.modelSettings();
      if (!mounted) return;
      setState(() {
        _settings = settings;
        _loadingSettings = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loadingSettings = false;
      });
    }
  }

  void _schedule(String value) {
    _debounce?.cancel();
    final text = value.trim();
    if (text.length < 2) {
      setState(() {
        _page = null;
        _saveError = null;
        _loading = false;
        if (_settings != null) {
          _error = null;
        }
      });
      return;
    }
    _debounce = Timer(const Duration(milliseconds: 350), () => _search(text));
  }

  Future<void> _search(String raw) async {
    final text = raw.trim();
    if (text.length < 2) return;
    final ticket = ++_ticket;
    setState(() {
      _loading = true;
      _saveError = null;
    });
    try {
      final page = await widget.api.searchModels(text);
      if (!mounted || ticket != _ticket) return;
      setState(() {
        _page = page;
        _loading = false;
        _error = null;
      });
    } on ApiException catch (e) {
      if (!mounted || ticket != _ticket) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    }
  }

  Future<void> _choose(String model, String provider) async {
    final key = '$model|$provider';
    setState(() {
      _saving = key;
      _saveError = null;
    });
    try {
      final next = await widget.api.saveModel(model: model, provider: provider);
      if (!mounted) return;
      setState(() {
        _settings = next;
        _saving = null;
      });
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Модель сохранена. Следующее ревью пойдёт в неё.'),
        ),
      );
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _saving = null;
        _saveError = e.message;
      });
    }
  }

  Future<void> _reset() async {
    setState(() {
      _resetting = true;
      _saveError = null;
    });
    try {
      final next = await widget.api.resetModel();
      if (!mounted) return;
      setState(() {
        _settings = next;
        _resetting = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _resetting = false;
        _saveError = e.message;
      });
    }
  }

  bool _isCurrent(String model, String provider) {
    final settings = _settings;
    if (settings == null) return false;
    return settings.model == model && settings.provider == provider;
  }

  @override
  Widget build(BuildContext context) {
    final wide = isWide(context);
    final settings = _settings;
    return ListView(
      padding: EdgeInsets.fromLTRB(wide ? 28 : 16, 20, wide ? 28 : 16, 32),
      children: [
        const Text(
          'Настройки',
          style: TextStyle(fontSize: 28, fontWeight: FontWeight.w700, letterSpacing: -0.6),
        ),
        const SizedBox(height: 6),
        const Text(
          'Поиск по актуальным текстовым моделям OpenRouter. В карточке — поставщик, сбор данных, полная цена, задержка, доступность и бенчмарк.',
          style: TextStyle(color: AppColors.muted, height: 1.35),
        ),
        const SizedBox(height: 18),
        if (_loadingSettings && settings == null)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(child: CircularProgressIndicator(color: AppColors.accent)),
          )
        else if (settings != null)
          _CurrentModel(settings: settings, resetting: _resetting, onReset: _reset),
        if (settings != null && !settings.ai) ...[
          const SizedBox(height: 12),
          const Text(
            'Ключ OPENROUTER_API_KEY не задан: ревью и пояснение поиска выключены. Каталог при этом открывается, но задержка поставщика может быть пустой.',
            style: TextStyle(color: AppColors.warn, height: 1.35),
          ),
        ],
        const SizedBox(height: 16),
        TextField(
          controller: _query,
          onChanged: _schedule,
          onSubmitted: _search,
          textInputAction: TextInputAction.search,
          decoration: InputDecoration(
            hintText: 'claude, gpt-4o-mini, gemini',
            suffixIcon: _loading
                ? const Padding(
                    padding: EdgeInsets.all(12),
                    child: SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2, color: AppColors.accent),
                    ),
                  )
                : IconButton(
                    tooltip: 'Искать',
                    onPressed: () => _search(_query.text),
                    icon: const Icon(Icons.search),
                  ),
          ),
        ),
        const SizedBox(height: 8),
        const Text(
          'Цены — тариф поставщика в USD. Комиссия вашего плана OpenRouter в них не входит.',
          style: TextStyle(color: AppColors.muted, fontSize: 13, height: 1.35),
        ),
        if (_error != null) ...[
          const SizedBox(height: 12),
          Text(_error!, style: const TextStyle(color: AppColors.bad)),
          if (settings == null) ...[
            const SizedBox(height: 8),
            OutlinedButton(onPressed: _loadSettings, child: const Text('Повторить')),
          ],
        ],
        if (_saveError != null) ...[
          const SizedBox(height: 12),
          Text(_saveError!, style: const TextStyle(color: AppColors.bad)),
        ],
        if (_page != null) ...[
          const SizedBox(height: 16),
          _Results(
            page: _page!,
            saving: _saving,
            isCurrent: _isCurrent,
            onChoose: _choose,
          ),
        ],
        const SizedBox(height: 28),
        const Divider(color: AppColors.line),
        const SizedBox(height: 8),
        ListTile(
          contentPadding: EdgeInsets.zero,
          leading: const Icon(Icons.password),
          title: const Text('Сменить пароль'),
          onTap: widget.onChangePassword,
        ),
        ListTile(
          contentPadding: EdgeInsets.zero,
          leading: const Icon(Icons.logout),
          title: const Text('Выйти'),
          onTap: widget.onLogout,
        ),
      ],
    );
  }
}

class _CurrentModel extends StatelessWidget {
  const _CurrentModel({
    required this.settings,
    required this.resetting,
    required this.onReset,
  });

  final ModelSettings settings;
  final bool resetting;
  final VoidCallback onReset;

  @override
  Widget build(BuildContext context) {
    return Panel(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('Сейчас', style: TextStyle(color: AppColors.muted, fontSize: 13)),
          const SizedBox(height: 6),
          SelectableText(
            settings.model,
            style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 6),
          Text(
            '${settings.providerLabel} · ${settings.sourceLabel}',
            style: const TextStyle(height: 1.35),
          ),
          if (settings.source == 'settings') ...[
            const SizedBox(height: 4),
            Text(
              'Переменная окружения: ${settings.envModel}',
              style: const TextStyle(color: AppColors.muted, fontSize: 13),
            ),
            const SizedBox(height: 12),
            OutlinedButton(
              onPressed: resetting ? null : onReset,
              child: Text(resetting ? 'Сбрасываю…' : 'Вернуть OPENROUTER_MODEL'),
            ),
          ],
        ],
      ),
    );
  }
}

class _Results extends StatelessWidget {
  const _Results({
    required this.page,
    required this.saving,
    required this.isCurrent,
    required this.onChoose,
  });

  final CatalogPage page;
  final String? saving;
  final bool Function(String model, String provider) isCurrent;
  final Future<void> Function(String model, String provider) onChoose;

  @override
  Widget build(BuildContext context) {
    if (page.results.isEmpty) {
      return const Text('Ничего не нашлось. Попробуйте id или часть названия.');
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (page.truncated)
          Padding(
            padding: const EdgeInsets.only(bottom: 10),
            child: Text(
              'Показаны ${page.results.length} из ${page.matchCount}. Уточните запрос.',
              style: const TextStyle(color: AppColors.muted),
            ),
          ),
        if (page.latencyNeedsKey)
          const Padding(
            padding: EdgeInsets.only(bottom: 10),
            child: Text(
              'Задержку и скорость OpenRouter отдаёт только вместе с ключом.',
              style: TextStyle(color: AppColors.muted, fontSize: 13),
            ),
          ),
        if (!page.dataPolicyLoaded)
          const Padding(
            padding: EdgeInsets.only(bottom: 10),
            child: Text(
              'Политику сбора данных OpenRouter сейчас не отдал.',
              style: TextStyle(color: AppColors.warn, fontSize: 13),
            ),
          ),
        for (final model in page.results) ...[
          _ModelCard(
            model: model,
            saving: saving,
            isCurrent: isCurrent,
            onChoose: onChoose,
            latencyNeedsKey: page.latencyNeedsKey,
          ),
          const SizedBox(height: 12),
        ],
      ],
    );
  }
}

class _ModelCard extends StatelessWidget {
  const _ModelCard({
    required this.model,
    required this.saving,
    required this.isCurrent,
    required this.onChoose,
    required this.latencyNeedsKey,
  });

  final CatalogModel model;
  final String? saving;
  final bool Function(String model, String provider) isCurrent;
  final Future<void> Function(String model, String provider) onChoose;
  final bool latencyNeedsKey;

  @override
  Widget build(BuildContext context) {
    return Panel(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(model.name, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
          const SizedBox(height: 4),
          SelectableText(
            model.id,
            style: const TextStyle(color: AppColors.muted, fontSize: 13),
          ),
          if (model.contextLength > 0) ...[
            const SizedBox(height: 8),
            Text('Контекст ${model.contextLength} токенов'),
          ],
          if (model.priceHint.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(model.priceHint),
          ],
          const SizedBox(height: 10),
          if (model.benchmarkLines.isEmpty)
            Text(model.benchmarkNote, style: const TextStyle(color: AppColors.muted))
          else ...[
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: [
                for (final line in model.benchmarkLines) MetaChip(text: line),
              ],
            ),
            if (model.benchmarkNote.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(
                model.benchmarkNote,
                style: const TextStyle(color: AppColors.muted, fontSize: 12, height: 1.35),
              ),
            ],
          ],
          if (model.description.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(model.description, style: const TextStyle(height: 1.35)),
          ],
          const SizedBox(height: 12),
          _ChooseRow(
            title: 'Автовыбор OpenRouter',
            subtitle: 'Поставщик не фиксируется. Кто получит запрос и соберёт ли он данные, решит маршрутизатор.',
            selected: isCurrent(model.id, ''),
            busy: saving == '${model.id}|',
            onPressed: () => onChoose(model.id, ''),
          ),
          if (model.providersError.isNotEmpty) ...[
            const SizedBox(height: 10),
            Text(model.providersError, style: const TextStyle(color: AppColors.warn)),
          ],
          if (model.providers.isEmpty && model.pricingLines.isNotEmpty) ...[
            const SizedBox(height: 10),
            const Text('Цена из каталога модели', style: TextStyle(fontWeight: FontWeight.w600)),
            const SizedBox(height: 4),
            for (final line in model.pricingLines) _Detail(line),
            for (final line in model.overrideLines) _Detail(line),
          ],
          for (final provider in model.providers) ...[
            const SizedBox(height: 10),
            _ProviderCard(
              provider: provider,
              selected: isCurrent(model.id, provider.tag),
              busy: saving == '${model.id}|${provider.tag}',
              latencyNeedsKey: latencyNeedsKey,
              onPressed: provider.tag.isEmpty ? null : () => onChoose(model.id, provider.tag),
            ),
          ],
        ],
      ),
    );
  }
}

class _ProviderCard extends StatelessWidget {
  const _ProviderCard({
    required this.provider,
    required this.selected,
    required this.busy,
    required this.latencyNeedsKey,
    required this.onPressed,
  });

  final CatalogProvider provider;
  final bool selected;
  final bool busy;
  final bool latencyNeedsKey;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    final facts = <String>[
      if (provider.tag.isNotEmpty) provider.tag,
      if (provider.quantization.isNotEmpty) provider.quantization,
      if (provider.serviceTier.isNotEmpty) 'тариф ${provider.serviceTier}',
      if (provider.contextLength > 0) 'контекст ${provider.contextLength}',
      if (provider.maxCompletionTokens != null) 'выход до ${provider.maxCompletionTokens}',
    ];
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: AppColors.surface2,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: selected ? AppColors.accent : AppColors.line),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(provider.name, style: const TextStyle(fontWeight: FontWeight.w700)),
          if (facts.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(facts.join(' · '), style: const TextStyle(color: AppColors.muted, fontSize: 13)),
          ],
          const SizedBox(height: 8),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              MetaChip(text: provider.availability, color: _availabilityColor(provider.availability)),
              MetaChip(text: _dataChip(provider.collectsData), color: _dataColor(provider.collectsData)),
            ],
          ),
          const SizedBox(height: 8),
          if (provider.dataLine.isNotEmpty) _Detail(provider.dataLine),
          if (provider.uptimeLine.isNotEmpty) _Detail(provider.uptimeLine),
          if (provider.latencyLine.isNotEmpty)
            _Detail(provider.latencyLine)
          else if (!latencyNeedsKey)
            const _Detail('Свежих замеров задержки нет.'),
          if (provider.throughputLine.isNotEmpty) _Detail(provider.throughputLine),
          if (provider.pricingLines.isNotEmpty) ...[
            const SizedBox(height: 6),
            const Text('Цена', style: TextStyle(fontWeight: FontWeight.w600)),
            const SizedBox(height: 2),
            for (final line in provider.pricingLines) _Detail(line),
          ],
          for (final line in provider.overrideLines) _Detail(line),
          const SizedBox(height: 8),
          const Text(
            'Ревью пойдёт только к этому поставщику. Если он недоступен, запрос не уйдёт к другому.',
            style: TextStyle(color: AppColors.muted, fontSize: 12, height: 1.35),
          ),
          const SizedBox(height: 10),
          if (onPressed == null)
            const Text(
              'У этого варианта нет идентификатора маршрута, выбрать его нельзя.',
              style: TextStyle(color: AppColors.muted, fontSize: 13),
            )
          else
            FilledButton(
              onPressed: selected || busy ? null : onPressed,
              child: Text(selected ? 'Выбрано' : (busy ? 'Сохраняю…' : 'Выбрать')),
            ),
        ],
      ),
    );
  }
}

class _ChooseRow extends StatelessWidget {
  const _ChooseRow({
    required this.title,
    required this.subtitle,
    required this.selected,
    required this.busy,
    required this.onPressed,
  });

  final String title;
  final String subtitle;
  final bool selected;
  final bool busy;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: AppColors.surface2,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: selected ? AppColors.accent : AppColors.line),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title, style: const TextStyle(fontWeight: FontWeight.w700)),
          const SizedBox(height: 4),
          Text(subtitle, style: const TextStyle(color: AppColors.muted, fontSize: 13, height: 1.35)),
          const SizedBox(height: 10),
          FilledButton(
            onPressed: selected || busy ? null : onPressed,
            child: Text(selected ? 'Выбрано' : (busy ? 'Сохраняю…' : 'Выбрать')),
          ),
        ],
      ),
    );
  }
}

class _Detail extends StatelessWidget {
  const _Detail(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 2),
      child: Text(text, style: const TextStyle(fontSize: 13, height: 1.35)),
    );
  }
}

Color _availabilityColor(String value) {
  if (value == 'доступен') return AppColors.ok;
  if (value == 'деградация') return AppColors.bad;
  return AppColors.muted;
}

String _dataChip(bool? collects) {
  if (collects == null) return 'политика неизвестна';
  if (collects) return 'собирает данные';
  return 'не собирает данные';
}

Color _dataColor(bool? collects) {
  if (collects == null) return AppColors.muted;
  if (collects) return AppColors.warn;
  return AppColors.ok;
}
