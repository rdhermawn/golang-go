<?php

require_once('./api/PwAPI.php');
require('./configs/config.php');

$api = new API();

function sendMessageToDiscordWebhook($webhookUrl, $messageContent)
{
    try {
        $curl = curl_init($webhookUrl);

        $payload = json_encode(['content' => $messageContent]);
        curl_setopt($curl, CURLOPT_POST, true);
        curl_setopt($curl, CURLOPT_POSTFIELDS, $payload);
        curl_setopt($curl, CURLOPT_HTTPHEADER, [
            'Content-Type: application/json'
        ]);
        curl_setopt($curl, CURLOPT_RETURNTRANSFER, true);

        $result = curl_exec($curl);

        if ($result === false) {
            throw new Exception('Error sending message to Discord via webhook: ' . curl_error($curl));
        }

        curl_close($curl);
        return $result;
    } catch (\Exception $e) {
        echo 'An error occurred: ' . $e->getMessage();
    }
}

$argv[1]($argv[2]);

function convertChineseToPortuguese($text) {
    $replacements = [
        '成功' => 'SUCCESS',
        '材料消失' => 'FAILURE', 
        '属性爆掉' => 'RESET',
        '属性降低一级' => 'DOWNGRADED'
    ];
    
    return str_replace(array_keys($replacements), array_values($replacements), $text);
}

function getItemName($itemId) {
    $filePath = './RAE_Exported_Table.tab';
    
    if (!file_exists($filePath)) {
        return "Item {$itemId}";
    }
    
    $file = fopen($filePath, 'r');
    if (!$file) {
        return "Item {$itemId}";
    }
    
    while (($line = fgets($file)) !== false) {
        $columns = explode("\t", trim($line));
        
        if (count($columns) >= 3 && $columns[0] === $itemId) {
            fclose($file);
            $itemName = preg_replace('/^[☆★✦✧✩✪✫✬✭✮✯✰✱✲✳✴✵✶✷✸✹✺✻✼✽✾✿❀❁❂❃❄❅❆❇❈❉❊❋❌❍❎❏❐❑❒❓❔❕❖❗❘❙❚❛❜❝❞❟❠❡❢❣❤❥❦❧❨❩❪❫❬❭❮❯❰❱❲❳❴❵❶❷❸❹❺❻❼❽❾❿➀➁➂➃➄➅➆➇➈➉➊➋➌➍➎➏➐➑➒➓➔→↔↕➘➙➚➛➜➝➞➟➠➡➢➣➤➥➦➧➨➩➪➫➬➭➮➯➰➱➲➳➴➵➶➷➸➹➺➻➼➽➾➿⟀⟁⟂⟃⟄⟅⟆⟇⟈⟉⟊⟋⟌⟍⟎⟏⟐⟑⟒⟓⟔⟕⟖⟗⟘⟙⟚⟛⟜⟝⟞⟟⟠⟡⟢⟣⟤⟥⟦⟧⟨⟩⟪⟫⟬⟭⟮⟯⟰⟱⟲⟳⟴⟵⟶⟷⟸⟹⟺⟻⟼⟽⟾⟿]*/', '', $columns[2]);
            return trim($itemName) ?: "Item {$itemId}";
        }
    }
    
    fclose($file);
    return "Item {$itemId}";
}

function getStoneEmoticon($stoneId) {
    if ($stoneId === '-1') {
        return '';
    }

    $stoneEmoticons = [
        '15049' => '<:ceu:1397026134243147869>',
        '12751' => '<:maligna:1397026138760679484>',
        '15692' => '<:ceu:1397026134243147869>',
        '12980' => '<:ceuterra:1397026136353144842>',
    ];

    return isset($stoneEmoticons[$stoneId]) ? $stoneEmoticons[$stoneId] : "Material {$stoneId}";
}

function processLogLine($line = null) {
    global $api, $config;

    $pattern = '/用户(\d+)精炼物品(\d+)\[(成功|材料消失|属性爆掉|属性降低一级)\]，精炼前级别(\d+) 消耗幻仙石(\d+) 概率物品(-1|\d+)/u';

    if (preg_match($pattern, $line, $matches)) {
        $roleId = $matches[1];
        $itemId = $matches[2];
        $resultRaw = $matches[3];
        $result = convertChineseToPortuguese($resultRaw);
        $level = intval($matches[4]);
        $stoneCount = $matches[5];
        $stoneId = $matches[6];

        $roleInfo = $api->getRoleBase($roleId);
        $playerName = $roleInfo ? $roleInfo['name'] : 'Unknown Player';

        $resultEmojis = [
            'SUCCESS' => '✅',
            'FAILURE' => '❌',
            'RESET' => '💥',
            'DOWNGRADED' => '⬇️'
        ];

        $emoji = $resultEmojis[$result] ?? '🔧';
        $itemName = getItemName($itemId);
        $materialDisplay = getStoneEmoticon($stoneId);
        $targetLevel = $level + 1;

        $materialText = $materialDisplay ? "using {$materialDisplay}" : "";

        switch ($result) {
            case 'SUCCESS':
                $mensagem = "{$emoji} **{$playerName}** refined **{$itemName}** to +{$targetLevel} {$materialText}";
                break;

            case 'FAILURE':
                $mensagem = "{$emoji} **{$playerName}** tried to refine **{$itemName}** to +{$targetLevel}, but it dropped to +{$level} {$materialText}";
                break;

            case 'RESET':
                $mensagem = "{$emoji} **{$playerName}** tried to refine **{$itemName}** to +{$targetLevel}, but it was reset to +0 {$materialText}";
                break;

            case 'DOWNGRADED':
                $downgradedLevel = max($level - 1, 0);
                $mensagem = "{$emoji} **{$playerName}** tried to refine **{$itemName}** to +{$targetLevel}, but it was downgraded to +{$downgradedLevel} {$materialText}";
                break;

            default:
                $mensagem = "{$emoji} **{$playerName}** refined **{$itemName}** from +{$level} {$materialText} ({$result})";
        }

        sendMessageToDiscordWebhook($config['discord']['webhook_url'], $mensagem);
    }
}

?>
